package segcan

import (
	"github.com/knieriem/modbus/netconn"
	seg "github.com/knieriem/seg/modbus"
)

func init() {
	netconn.RegisterProtocol(&netconn.Proto{
		Name:           "seg/can",
		RequiredFields: netconn.CanIDFields,
		OptionalFields: netconn.DevFields,
		Dial:           dial,
		InterfaceGroup: &canAdapters,
	})
}

func dial(cf *netconn.Conf) (conn *netconn.Conn, err error) {
	f, err := openCAN(cf)
	if err != nil {
		return
	}

	name := f.dev.Name()
	id := name.String()
	f.dev = devWrapper.wrap(f.dev, id)
	nc := seg.NewNetConn(f, 8, "can")

	conn = &netconn.Conn{
		Addr:       cf.MakeAddr(id, true),
		Device:     id,
		DeviceInfo: name.Format("\t(", ",", ")"),
		NetConn:    nc,
		Closer:     f,
		ExitC:      nc.ExitC,
	}
	return
}
