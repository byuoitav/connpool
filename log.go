package connpool

type Logger interface {
	Debugf(format string, a ...interface{})
	Infof(format string, a ...interface{})
	Warnf(format string, a ...interface{})
	Errorf(format string, a ...interface{})
}

func (p *Pool) debugf(format string, a ...interface{}) {
	if p.Logger != nil {
		p.Logger.Debugf(format, a...)
	}
}

func (p *Pool) infof(format string, a ...interface{}) {
	if p.Logger != nil {
		p.Logger.Infof(format, a...)
	}
}

func (p *Pool) warnf(format string, a ...interface{}) {
	if p.Logger != nil {
		p.Logger.Warnf(format, a...)
	}
}

func (p *Pool) errorf(format string, a ...interface{}) {
	if p.Logger != nil {
		p.Logger.Errorf(format, a...)
	}
}
