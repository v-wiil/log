package log

import (
	"net"
	"strconv"
	"sync"
	"time"
)

// SyslogWriter is an Writer that writes logs to a syslog server..
type SyslogWriter struct {
	// Network specifies network of the syslog server
	Network string

	// Address specifies address of the syslog server
	Address string

	// Hostname specifies hostname of the syslog message
	Hostname string

	// Tag specifies tag of the syslog message
	Tag string

	// Marker specifies prefix of the syslog message, e.g. `@cee:`
	Marker string

	// Dial specifies the dial function for creating TCP/TLS connections.
	Dial func(network, addr string) (net.Conn, error)

	mu    sync.Mutex
	conn  net.Conn
	local bool
}

// Close closes a connection to the syslog server.
func (w *SyslogWriter) Close() (err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.conn != nil {
		err = w.conn.Close()
		w.conn = nil
		return
	}
	return
}

// connect makes a connection to the syslog server.
func (w *SyslogWriter) connect() (err error) {
	if w.conn != nil {
		w.conn.Close()
		w.conn = nil
	}

	var dial = w.Dial
	if dial == nil {
		dial = net.Dial
	}

	w.conn, err = dial(w.Network, w.Address)
	if err != nil {
		return
	}

	w.local = w.Address != "" && w.Address[0] == '/'

	if w.Hostname == "" {
		if w.local {
			w.Hostname = hostname
		} else {
			w.Hostname = w.conn.LocalAddr().String()
		}
	}

	return
}

// WriteEntry implements Writer, sends logs with priority to the syslog server.
func (w *SyslogWriter) WriteEntry(e *Entry) (n int, err error) {
	if w.conn == nil {
		w.mu.Lock()
		if w.conn == nil {
			err = w.connect()
			if err != nil {
				w.mu.Unlock()
				return
			}
		}
		w.mu.Unlock()
	}

	// convert level to syslog priority
	var priority byte
	switch e.Level {
	case TraceLevel:
		priority = '7' // LOG_DEBUG
	case DebugLevel:
		priority = '7' // LOG_DEBUG
	case InfoLevel:
		priority = '6' // LOG_INFO
	case WarnLevel:
		priority = '4' // LOG_WARNING
	case ErrorLevel:
		priority = '3' // LOG_ERR
	case FatalLevel:
		priority = '2' // LOG_CRIT
	case PanicLevel:
		priority = '1' // LOG_ALERT
	default:
		priority = '6' // LOG_INFO
	}

	e1 := epool.Get().(*Entry)
	defer epool.Put(e1)

	// <PRI>TIMESTAMP HOSTNAME TAG[PID]: MSG
	b := append(e1.buf[:0], '<', priority, '>')
	if w.local {
		// Compared to the network form below, the changes are:
		//	1. Use time.Stamp instead of time.RFC3339.
		//	2. Drop the hostname field.
		b = timeNow().AppendFormat(b, time.Stamp)
	} else {
		b = timeNow().AppendFormat(b, time.RFC3339)
		b = append(b, ' ')
		b = append(b, w.Hostname...)
	}
	b = append(b, ' ')
	b = append(b, w.Tag...)
	b = append(b, '[')
	b = strconv.AppendInt(b, int64(pid), 10)
	b = append(b, ']', ':', ' ')
	b = append(b, w.Marker...)
	b = append(b, e.buf...)
	e1.buf = b

	w.mu.Lock()
	defer w.mu.Unlock()

	if w.conn != nil {
		if n, err := w.conn.Write(b); err == nil {
			return n, err
		}
	}
	if err := w.connect(); err != nil {
		return 0, err
	}
	return w.conn.Write(b)
}

var _ Writer = (*SyslogWriter)(nil)
