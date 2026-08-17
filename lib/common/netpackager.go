package common

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strconv"
)

type NetPackager interface {
	Pack(writer io.Writer) (err error)
	UnPack(reader io.Reader) (err error)
}

const (
	ipV4       = 1
	domainName = 3
	ipV6       = 4
)

type UDPHeader struct {
	Rsv  uint16
	Frag uint8
	Addr *Addr
}

func NewUDPHeader(rsv uint16, frag uint8, addr *Addr) *UDPHeader {
	return &UDPHeader{
		Rsv:  rsv,
		Frag: frag,
		Addr: addr,
	}
}

type Addr struct {
	Type uint8
	Host string
	Port uint16
}

func (addr *Addr) String() string {
	return net.JoinHostPort(addr.Host, strconv.Itoa(int(addr.Port)))
}

func (addr *Addr) Decode(b []byte) error {
	if len(b) < 1 {
		return errors.New("decode error: missing address type")
	}
	addr.Type = b[0]
	pos := 1
	switch addr.Type {
	case ipV4:
		if len(b) < pos+net.IPv4len+2 {
			return errors.New("decode error: truncated IPv4 address")
		}
		addr.Host = net.IP(b[pos : pos+net.IPv4len]).String()
		pos += net.IPv4len
	case ipV6:
		if len(b) < pos+net.IPv6len+2 {
			return errors.New("decode error: truncated IPv6 address")
		}
		addr.Host = net.IP(b[pos : pos+net.IPv6len]).String()
		pos += net.IPv6len
	case domainName:
		if len(b) < pos+1 {
			return errors.New("decode error: missing domain length")
		}
		addrlen := int(b[pos])
		pos++
		if addrlen == 0 || len(b) < pos+addrlen+2 {
			return errors.New("decode error: invalid domain address")
		}
		addr.Host = string(b[pos : pos+addrlen])
		pos += addrlen
	default:
		return errors.New("decode error")
	}

	addr.Port = binary.BigEndian.Uint16(b[pos:])

	return nil
}

func (addr *Addr) Encode(b []byte) (int, error) {
	if len(b) < 1 {
		return 0, errors.New("encode error: buffer is too small")
	}
	b[0] = addr.Type
	pos := 1
	switch addr.Type {
	case ipV4:
		if len(b) < pos+net.IPv4len+2 {
			return 0, errors.New("encode error: buffer is too small")
		}
		ip4 := net.ParseIP(addr.Host).To4()
		if ip4 == nil {
			ip4 = net.IPv4zero.To4()
		}
		pos += copy(b[pos:], ip4)
	case domainName:
		if len(addr.Host) == 0 || len(addr.Host) > 255 || len(b) < pos+1+len(addr.Host)+2 {
			return 0, errors.New("encode error: invalid domain address")
		}
		// #nosec G115 -- the domain length is explicitly capped at 255 above.
		b[pos] = byte(len(addr.Host))
		pos++
		pos += copy(b[pos:], []byte(addr.Host))
	case ipV6:
		if len(b) < pos+net.IPv6len+2 {
			return 0, errors.New("encode error: buffer is too small")
		}
		ip16 := net.ParseIP(addr.Host).To16()
		if ip16 == nil {
			ip16 = net.IPv6zero.To16()
		}
		pos += copy(b[pos:], ip16)
	default:
		if len(b) < pos+net.IPv4len+2 {
			return 0, errors.New("encode error: buffer is too small")
		}
		b[0] = ipV4
		copy(b[pos:pos+4], net.IPv4zero.To4())
		pos += 4
	}
	binary.BigEndian.PutUint16(b[pos:], addr.Port)
	pos += 2

	return pos, nil
}

func (h *UDPHeader) Write(w io.Writer) error {
	b := BufPoolUdp.Get().([]byte)
	defer BufPoolUdp.Put(b)

	binary.BigEndian.PutUint16(b[:2], h.Rsv)
	b[2] = h.Frag

	addr := h.Addr
	if addr == nil {
		addr = &Addr{}
	}
	length, err := addr.Encode(b[3:])
	if err != nil {
		return err
	}

	_, err = w.Write(b[:3+length])
	return err
}

type UDPDatagram struct {
	Header *UDPHeader
	Data   []byte
}

func ReadUDPDatagram(r io.Reader) (*UDPDatagram, error) {
	b := BufPoolUdp.Get().([]byte)
	defer BufPoolUdp.Put(b)

	// when r is a streaming (such as TCP connection), we may read more than the required data,
	// but we don't know how to handle it. So we use io.ReadFull to instead of io.ReadAtLeast
	// to make sure that no redundant data will be discarded.
	n, err := io.ReadFull(r, b[:5])
	if err != nil {
		return nil, err
	}

	header := &UDPHeader{
		Rsv:  binary.BigEndian.Uint16(b[:2]),
		Frag: b[2],
	}

	atype := b[3]
	hlen := 0
	switch atype {
	case ipV4:
		hlen = 10
	case ipV6:
		hlen = 22
	case domainName:
		hlen = 7 + int(b[4])
	default:
		return nil, errors.New("addr not support")
	}
	if hlen < n || hlen > len(b) {
		return nil, errors.New("invalid udp header length")
	}
	dlen := int(header.Rsv)
	if dlen == 0 { // standard SOCKS5 UDP datagram
		remaining := len(b) - n
		extra, err := io.ReadAll(io.LimitReader(r, int64(remaining+1)))
		if err != nil {
			return nil, err
		}
		if len(extra) > remaining {
			return nil, errors.New("udp datagram is too large")
		}
		copy(b[n:], extra)
		n += len(extra) // total length
		if n < hlen {
			return nil, io.ErrUnexpectedEOF
		}
		dlen = n - hlen // data length
	} else { // extended feature, for UDP over TCP, using reserved field as data length
		total := hlen + dlen
		if total < n || total > len(b) {
			return nil, errors.New("invalid udp payload length")
		}
		if _, err := io.ReadFull(r, b[n:total]); err != nil {
			return nil, err
		}
		n = total
	}
	header.Addr = new(Addr)
	if err := header.Addr.Decode(b[3:hlen]); err != nil {
		return nil, err
	}
	data := make([]byte, dlen)
	copy(data, b[hlen:n])
	d := &UDPDatagram{
		Header: header,
		Data:   data,
	}
	return d, nil
}

func NewUDPDatagram(header *UDPHeader, data []byte) *UDPDatagram {
	return &UDPDatagram{
		Header: header,
		Data:   data,
	}
}

func (d *UDPDatagram) Write(w io.Writer) error {
	h := d.Header
	if h == nil {
		h = &UDPHeader{}
	}
	buf := bytes.Buffer{}
	if err := h.Write(&buf); err != nil {
		return err
	}
	if _, err := buf.Write(d.Data); err != nil {
		return err
	}

	_, err := buf.WriteTo(w)
	return err
}

func ToSocksAddr(addr net.Addr) *Addr {
	host := "0.0.0.0"
	var port uint16
	if addr != nil {
		h, p, err := net.SplitHostPort(addr.String())
		if err == nil {
			host = h
			parsed, parseErr := strconv.ParseUint(p, 10, 16)
			if parseErr == nil {
				// #nosec G115 -- ParseUint with bitSize 16 bounds the conversion.
				port = uint16(parsed)
			}
		}
	}
	return &Addr{
		Type: ipV4,
		Host: host,
		Port: port,
	}
}
