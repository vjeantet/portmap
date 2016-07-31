package portmap

import "net"
import "time"
import "github.com/vjeantet/portmap/ssdp"
import "github.com/vjeantet/portmap/upnp"

import "github.com/hlandau/xlog"

var log, Log = xlog.New("portmap")

type mode int

func (m *mapping) portMappingLoop(gwa []net.IP) {
	aborting := false
	var ok bool
	var d time.Duration
	for {
		// Already inactive (e.g. expired or was never active), so no need to do anything.
		if aborting && !m.lIsActive() {
			return
		}

		svcs := ssdp.GetServicesByType(upnpWANIPConnectionURN)
		if len(svcs) == 0 {
			continue
		}

		ok = m.tryUPnP(svcs, aborting)
		d = 1 * time.Hour

		// If we are aborting, then the call we just made was to remove the mapping,
		// not set it, and we're done.
		if aborting {
			m.setInactive()
			return
		}

		// Backoff
		if ok {
			m.cfg.Backoff.Reset()
		} else {
			// failed, do retry delay
			d = m.cfg.Backoff.NextDelay()
			if d == 0 {
				// max tries occurred
				m.setInactive()
				return
			}
		}

		m.notify()

		select {
		case <-m.abortChan:
			aborting = true
			m.tryUPnP(svcs, true)

		case <-time.After(d):
			// wait until we need to renew
		}
	}
}

func (m *mapping) notify() {
	ea := m.ExternalAddr()

	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.prevValue == ea {
		// no change
		return
	}

	m.prevValue = ea

	select {
	case m.notifyChan <- struct{}{}:
	default:
	}
}

// UPnP

func (m *mapping) tryUPnP(svcs []ssdp.Service, destroy bool) bool {
	for _, svc := range svcs {
		if m.tryUPnPSvc(svc, destroy) {
			return true
		}
	}
	return false
}

const upnpWANIPConnectionURN = "urn:schemas-upnp-org:service:WANIPConnection:1"

func (m *mapping) tryUPnPSvc(svc ssdp.Service, destroy bool) bool {
	if destroy {
		// unmapping
		if !m.lIsActive() {
			return true
		}

		err := upnp.Unmap(svc.Location.String(), upnp.Protocol(m.cfg.Protocol), m.cfg.ExternalPort)
		return err == nil
	}

	// mapping
	actualExternalPort, err := upnp.Map(svc.Location.String(), upnp.Protocol(m.cfg.Protocol),
		m.cfg.InternalPort,
		m.cfg.ExternalPort, m.cfg.Name, m.cfg.Lifetime)

	if err != nil {
		return false
	}

	m.mutex.Lock()
	m.expireTime = time.Now().Add(m.cfg.Lifetime)
	m.cfg.ExternalPort = actualExternalPort
	m.mutex.Unlock()

	// Now attempt to get the external IP.
	if destroy {
		return true
	}

	extIP, err := upnp.GetExternalAddr(svc.Location.String())
	if err != nil {
		// mapping till succeeded
		return true
	}

	// update external address
	m.mutex.Lock()
	m.externalAddr = extIP.String()
	m.mutex.Unlock()

	return true
}

//

func (m *mapping) setInactive() {
	m.mutex.Lock()
	m.expireTime = time.Time{}
	m.mutex.Unlock()

	m.notify()
}
