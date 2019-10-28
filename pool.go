package pooled

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
)

type Work func(Conn) error

type Pool struct {
	ConnTTL         time.Duration
	TimeBetweenWork time.Duration
	NewConnection   func(context.Context) (net.Conn, error)
	Logger          Logger

	init sync.Once
	reqs chan request
}

type request struct {
	ctx  context.Context
	work Work
	resp chan error
}

func (p *Pool) Do(ctx context.Context, work Work) error {
	p.init.Do(func() {
		p.reqs = make(chan request)

		go func() {
			var conn Conn
			timer := time.NewTimer(p.ConnTTL)

			closeConn := func() {
				if conn != nil {
					conn.netconn().Close()
					conn = nil
				}
			}

			for {
				select {
				case req := <-p.reqs:
					// check if context is exceeded and we can skip this one
					if req.ctx.Err() != nil {
						continue
					}

					if conn == nil {
						p.Logger.Infof("Opening new connection")
						nconn, err := p.NewConnection(req.ctx)
						if err != nil {
							req.resp <- fmt.Errorf("failed to open new connection: %w", err)
							continue
						}

						conn = Wrap(nconn)
						p.Logger.Infof("Successfully opened new connection")
					} else {
						p.Logger.Infof("Reusing open connection")
					}

					// reset the buffer by reading everything currently in it
					bytes, err := conn.EmptyReadBuffer(p.ConnTTL)
					switch {
					case err != nil:
						req.resp <- fmt.Errorf("failed to empty buffer: %s", err)
						continue
					case len(bytes) > 0:
						p.Logger.Debugf("Read %v leftover bytes: 0x%x", len(bytes), bytes)
					}

					// reset the deadlines
					conn.netconn().SetDeadline(time.Time{})

					// do the work!
					err = req.work(conn)
					req.resp <- err

					// close the connection if necessary
					var nerr net.Error
					if errors.As(err, &nerr) && (!nerr.Temporary() || nerr.Timeout()) {
						// if it was a timeout error, close the connection
						p.Logger.Warnf("closing connection due to non-temporary or timeout error: %s", err.Error())
						closeConn()
						continue
					}

					// reset timer since we did something
					if !timer.Stop() {
						<-timer.C
					}
					timer.Reset(p.ConnTTL)
				case <-timer.C:
					p.Logger.Infof("Closing connection")

					closeConn()
				}
			}
		}()

		p.Logger.Infof("Started pool")
	})

	req := request{
		ctx:  ctx,
		work: work,
		resp: make(chan error),
	}

	p.reqs <- req

	select {
	case err := <-req.resp:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}