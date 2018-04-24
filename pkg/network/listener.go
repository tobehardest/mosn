package network

import (
	"context"
	"gitlab.alipay-inc.com/afe/mosn/pkg/api/v2"
	"gitlab.alipay-inc.com/afe/mosn/pkg/log"
	"gitlab.alipay-inc.com/afe/mosn/pkg/types"
	"net"
	"runtime/debug"
	"time"
)

// listener impl based on golang net package
type listener struct {
	name                                  string
	localAddress                          net.Addr
	bindToPort                            bool
	listenerTag                           uint64
	perConnBufferLimitBytes               uint32
	handOffRestoredDestinationConnections bool
	cb                                    types.ListenerEventListener
	rawl                                  *net.TCPListener
}

func NewListener(lc *v2.ListenerConfig) types.Listener {

	l := &listener{
		name:                                  lc.Name,
		localAddress:                          lc.Addr,
		bindToPort:                            lc.BindToPort,
		listenerTag:                           lc.ListenerTag,
		perConnBufferLimitBytes:               lc.PerConnBufferLimitBytes,
		handOffRestoredDestinationConnections: lc.HandOffRestoredDestinationConnections,
	}

	if lc.InheritListener != nil {
		//inherit old process's listener
		l.rawl = lc.InheritListener
	}

	return l
}

func (l *listener) Name() string {
	return l.name
}

func (l *listener) Addr() net.Addr {
	return l.localAddress
}

func (l *listener) Start(lctx context.Context) {
	//call listen if not inherit
	if l.rawl == nil {
		if err := l.listen(lctx); err != nil {
			// TODO: notify listener callbacks
			log.DefaultLogger.Fatalln(l.name, " listen failed, ", err)
			return
		}
	}

	if l.bindToPort {
		for {
			if err := l.accept(lctx); err != nil {
				if nerr, ok := err.(net.Error); ok && nerr.Timeout() {
					log.DefaultLogger.Infof("listener %s stop accepting connections by deadline", l.name)
					return
				} else if ope, ok := err.(*net.OpError); ok {
					if !(ope.Timeout() && ope.Temporary()) {
						log.DefaultLogger.Errorf("not temp-timeout error:%s", err.Error())
					}
				} else {
					log.DefaultLogger.Errorf("unknown error while listener accepting:%s", err.Error())
				}
			}
		}
	}
}

func (l *listener) Stop() {
	l.rawl.SetDeadline(time.Now())
}

func (l *listener) ListenerTag() uint64 {
	return l.listenerTag
}

func (l *listener) ListenerFD() (uintptr, error) {
	file, err := l.rawl.File()
	if err != nil {
		log.DefaultLogger.Println(l.name, " listener fd not found : ", err)
		return 0, err
	}
	return file.Fd(), nil
}

func (l *listener) PerConnBufferLimitBytes() uint32 {
	return l.perConnBufferLimitBytes
}

func (l *listener) SetListenerCallbacks(cb types.ListenerEventListener) {
	l.cb = cb
}

func (l *listener) Close(lctx context.Context) error {
	l.cb.OnClose()
	return l.rawl.Close()
}

func (l *listener) listen(lctx context.Context) error {
	var err error

	var rawl *net.TCPListener
	if rawl, err = net.ListenTCP("tcp", l.localAddress.(*net.TCPAddr)); err != nil {
		return err
	}

	l.rawl = rawl

	return nil
}

func (l *listener) accept(lctx context.Context) error {
	rawc, err := l.rawl.Accept()

	if err != nil {
		return err
	}

	// TODO: use thread pool
	go func() {
		defer func() {
			if p := recover(); p != nil {
				log.DefaultLogger.Errorf("panic %v\n", p)

				debug.PrintStack()
			}
		}()

		l.cb.OnAccept(rawc, l.handOffRestoredDestinationConnections)
	}()

	return nil
}
