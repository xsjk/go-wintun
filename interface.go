package wintun

import (
	"fmt"
	"net"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/xsjk/go-wintun/iphlpapi"
	"github.com/xsjk/go-wintun/kernel32"
	"golang.org/x/sys/windows"
	tun "golang.zx2c4.com/wintun"
)

type Interface struct {
	Name       string
	TunnelType string
	IP         string
	GUID       *windows.GUID

	adapter   *tun.Adapter
	session   tun.Session
	stopEvent windows.Handle
	readEvent windows.Handle
	channel   chan []byte
}

func Decode(data []byte) gopacket.Packet {
	var layerType gopacket.LayerType
	switch data[0] >> 4 {
	case 4:
		layerType = layers.LayerTypeIPv4
	case 6:
		layerType = layers.LayerTypeIPv6
	default:
		return nil
	}
	return gopacket.NewPacket(data, layerType, gopacket.DecodeOptions{Lazy: true, NoCopy: true})
}

func (t *Interface) Open() (err error) {

	ip, ipnet, err := net.ParseCIDR(t.IP)
	subnet, _ := ipnet.Mask.Size()
	if err != nil {
		return
	}

	t.adapter, err = tun.CreateAdapter(t.Name, t.TunnelType, t.GUID)
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			t.adapter.Close()
		}
	}()

	err = iphlpapi.SetAdapterIPv4(t.adapter.LUID(), ip.To4(), subnet)
	if err != nil {
		return
	}

	t.session, err = t.adapter.StartSession(0x400000)
	if err != nil {
		return
	}

	t.stopEvent, _ = kernel32.CreateEvent(true, false, "StopEvent")
	t.readEvent = t.session.ReadWaitEvent()

	t.channel = make(chan []byte)

	go func() {
		for {
			data, err := t.session.ReceivePacket()

			if err == nil {
				data_copy := make([]byte, len(data))
				copy(data_copy, data)
				t.session.ReleaseReceivePacket(data)
				t.channel <- data_copy

			} else {
				switch err {
				case windows.ERROR_NO_MORE_ITEMS:
					res, err := kernel32.WaitForMultipleObjects([]windows.Handle{t.readEvent, t.stopEvent}, false, windows.INFINITE)
					switch res {
					case windows.WAIT_OBJECT_0:
						continue
					case windows.WAIT_OBJECT_0 + 1:
						return
					default:
						fmt.Printf("WaitForMultipleObjects failed: %v\n", err)
					}
				case windows.ERROR_HANDLE_EOF:
					fmt.Printf("%v, you should set the stopEvent before closing the session\n", err)
					return
				default:
					fmt.Printf("Unexpected error: %d %v\n", err, err)
					return
				}
			}
		}
	}()

	return

}

func (t *Interface) Close() error {
	kernel32.SetEvent(t.stopEvent)
	defer kernel32.CloseHandle(t.stopEvent)
	t.session.End()
	return t.adapter.Close()
}

func (t *Interface) ReceiveAsync() <-chan []byte {
	return t.channel
}

func (t *Interface) Receive() []byte {
	return <-t.channel
}

func (t *Interface) Send(data []byte) (err error) {
	buffer, err := t.session.AllocateSendPacket(len(data))
	if err == nil {
		copy(buffer, data)
		t.session.SendPacket(buffer)
	}
	return
}

func (t *Interface) WaitForExit(duration uint32) bool {
	res, _ := kernel32.WaitForSingleObject(t.stopEvent, duration)
	switch res {
	case windows.WAIT_OBJECT_0:
		return true
	}
	return false
}
