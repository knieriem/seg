package segcan

import (
	"github.com/knieriem/modbus/netconn"
	"github.com/knieriem/seg"
	mod "github.com/knieriem/seg/modbus"
)

func init() {
	netconn.RegisterProtocol(&netconn.Proto{
		Name:           "seg/can",
		OptionalFields: netconn.DevFields | netconn.CanIDFields,
		Dial:           dial,
		InterfaceGroup: &canAdapters,
	})
}

func dial(cf *netconn.Conf) (conn *netconn.Conn, err error) {
	f, err := openCAN(cf)
	if err != nil {
		return
	}

	info := f.dev.Info()
	id := info.String()
	f.dev = devWrapper.wrap(f.dev, id)

	var opts []seg.Option
	if f.fdMode {
		opts = append(opts, seg.WithStrategy(seg.CANFDStrategy(f.segMax)))
	}
	nc := mod.NewNetConn(f, f.segMax, "can", opts...)

	conn = &netconn.Conn{
		Addr:       cf.MakeAddr(id, true),
		DeviceName: id,
		DeviceInfo: info.Format("\t(", ",", ")"),
		NetConn:    nc,
		Closer:     f,
		ExitC:      nc.ExitC,
	}
	return
}
