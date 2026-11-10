package srt

import (
	"net"

	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

// this is net.PacketConn with two additional methods
// that allow to get and set local IP.
type packetConn struct {
	net.PacketConn

	pc4 *ipv4.PacketConn
	pc6 *ipv6.PacketConn
}

func newPacketConn(wrapped net.PacketConn) *packetConn {
	c := &packetConn{
		PacketConn: wrapped,
	}

	// Enable PKTINFO to capture the destination IP of incoming packets.
	// Both pc4 and pc6 are attempted independently: on a dual-stack AF_INET6
	// socket pc6 delivers the destination (IPv4-mapped for IPv4 packets) while
	// pc4.WriteTo is still needed to pin the IPv4 source address on replies.
	pc6 := ipv6.NewPacketConn(wrapped)
	if err := pc6.SetControlMessage(ipv6.FlagDst, true); err == nil {
		c.pc6 = pc6
	}
	pc4 := ipv4.NewPacketConn(wrapped)
	if err := pc4.SetControlMessage(ipv4.FlagDst, true); err == nil {
		c.pc4 = pc4
	}

	return c
}

func (c *packetConn) readFromTo(buffer []byte) (int, net.Addr, net.IP, error) {
	switch {
	case c.pc6 != nil:
		n, cm, addr, err := c.pc6.ReadFrom(buffer)
		if cm != nil && cm.Dst != nil {
			// Normalize IPv4-mapped addresses (::ffff:x.x.x.x) to plain IPv4
			if ip4 := cm.Dst.To4(); ip4 != nil {
				return n, addr, ip4, err
			}
			return n, addr, cm.Dst, err
		}
		return n, addr, nil, err

	case c.pc4 != nil:
		n, cm, addr, err := c.pc4.ReadFrom(buffer)
		if cm != nil {
			return n, addr, cm.Dst, err
		}
		return n, addr, nil, err

	default:
		n, addr, err := c.ReadFrom(buffer)
		return n, addr, nil, err
	}
}

// writeToFrom sends buffer to remoteAddr, preferring to pin the source address
// to localAddr so a multi-homed host answers from the address the peer used.
//
// Pinning is best-effort. On some platforms the ancillary-data send is rejected
// for a destination whose family differs from the socket's -- notably an IPv4
// destination on a dual-stack AF_INET6 socket on Darwin, where sendmsg returns
// EINVAL. When that happens the packet is sent unpinned rather than dropped: an
// unpinned reply reaches the peer on any single-homed host, whereas a dropped
// one strands the handshake with no error anywhere.
func (c *packetConn) writeToFrom(buffer []byte, remoteAddr net.Addr, localAddr net.Addr) error {
	if localAddrUDP, ok := localAddr.(*net.UDPAddr); ok && localAddrUDP != nil {
		if _, ok := remoteAddr.(*net.UDPAddr); ok {
			// For IPv4 destinations use pc4 even on dual-stack sockets
			if ip4 := localAddrUDP.IP.To4(); ip4 != nil && c.pc4 != nil {
				if _, err := c.pc4.WriteTo(buffer, &ipv4.ControlMessage{Src: ip4}, remoteAddr); err == nil {
					return nil
				}
			} else if c.pc6 != nil {
				if _, err := c.pc6.WriteTo(buffer, &ipv6.ControlMessage{Src: localAddrUDP.IP}, remoteAddr); err == nil {
					return nil
				}
			}
		}
	}

	_, err := c.WriteTo(buffer, remoteAddr)
	return err
}
