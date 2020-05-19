package connpool

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
)

type NewConnectionFunc func(context.Context) (net.Conn, error)
type Work func(Conn) error

type Pool struct {
	NewConnection NewConnectionFunc
	TTL           time.Duration
	Delay         time.Duration
	Logger        Logger

	init     sync.Once
	requests chan request
}

type request struct {
	ctx  context.Context
	work Work
	resp chan error
}

func (p *Pool) Do(ctx context.Context, work Work) error {
	p.init.Do(func() {
		p.requests = make(chan request)

		go func() {
			var conn Conn
			timer := time.NewTimer(p.TTL)
			drained := false

			closeConn := func() {
				if conn != nil {
					err := conn.Close()
					if err != nil {
						p.errorf("unable to close connection: %s", err)
					}

					conn = nil
				}
			}

			for {
				select {
				case req := <-p.requests:
					// check if context is exceeded and we can skip this one
					if req.ctx.Err() != nil {
						continue
					}

					if conn == nil {
						p.infof("Opening new connection")

						// add a timeout (in case they didn't set one!) of the ttl
						ctx, cancel := context.WithTimeout(req.ctx, p.TTL)

						netConn, err := p.NewConnection(ctx)
						if err != nil {
							err = fmt.Errorf("failed to open new connection: %w", err)
							p.warnf(err.Error())
							req.resp <- err
							cancel()
							continue
						}

						cancel()
						conn = Wrap(netConn)

						p.infof("Successfully opened new connection")
					} else {
						p.infof("Reusing open connection")
					}

					deadline, ok := req.ctx.Deadline()
					if !ok {
						deadline = time.Now().Add(p.TTL)
					}

					// reset the buffer by reading everything currently in it
					bytes, err := conn.EmptyReadBuffer(deadline)
					switch {
					case err != nil:
						req.resp <- fmt.Errorf("failed to empty buffer: %s", err)
						continue
					case len(bytes) > 0:
						p.debugf("Read %v leftover bytes: 0x%x", len(bytes), bytes)
					}

					// reset the deadlines
					conn.SetDeadline(time.Time{})

					// do the work!
					err = req.work(conn)
					req.resp <- err

					// close the connection if necessary
					var netErr net.Error
					if errors.As(err, &netErr) && (!netErr.Temporary() || netErr.Timeout()) {
						// if it was a timeout error, close the connection
						p.warnf("closing connection due to non-temporary or timeout error: %s", err.Error())

						closeConn()
						continue
					}

					// reset timer since we did something
					if !timer.Stop() && !drained {
						<-timer.C
					}

					timer.Reset(p.TTL)
					drained = false

					// delay
					time.Sleep(p.Delay)
				case <-timer.C:
					drained = true
					p.infof("Closing connection")
					closeConn()
				}
			}
		}()

		p.infof("Started pool")
	})

	req := request{
		ctx:  ctx,
		work: work,
		resp: make(chan error),
	}

	p.requests <- req

	select {
	case err := <-req.resp:
		return err
	case <-ctx.Done():
		// drain the response channel whenever it comes back
		go func() {
			<-req.resp
		}()

		return fmt.Errorf("unable to do request: %w", ctx.Err())
	}
}
