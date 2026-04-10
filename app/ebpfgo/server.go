package ebpfgo

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"strings"
	"sync"
	"syscall"

	"github.com/cilium/ebpf/link"
	"github.com/suisrc/zgg/z"
	"github.com/suisrc/zgg/z/zc"
)

var C = struct {
	Ebpfgo Config `json:"ebpfgo"`
}{
	Ebpfgo: Config{
		MaxBodySize: -1,
	},
}

type InitFunc func(server z.Server, zgg *z.Zgg)

func init() {
	Init3(nil)
}

func Init3(ifn InitFunc) {
	z.Config(&C)

	flag.BoolVar(&C.Ebpfgo.Disabled, "e3disabled", false, "是否禁用 ebpfgo")
	flag.StringVar(&C.Ebpfgo.IfName, "e3ifname", "", "抓包网卡名称")
	flag.StringVar(&C.Ebpfgo.PcapRules, "e3pcap", "", "pcap 过滤表达式")
	flag.StringVar(&C.Ebpfgo.Direction, "e3direction", "", "流量方向: ingress|egress")
	flag.UintVar(&C.Ebpfgo.PID, "e3pid", 0, "PID 过滤值")
	flag.UintVar(&C.Ebpfgo.CPID, "e3cpid", 0, "容器 PID 过滤值")
	flag.Uint64Var(&C.Ebpfgo.CRID, "e3crid", 0, "容器命名空间过滤值")
	flag.StringVar(&C.Ebpfgo.Comm, "e3comm", "", "进程 comm 过滤")
	flag.StringVar(&C.Ebpfgo.SrcSpec, "e3src", "", "源地址 CIDR 过滤")
	flag.StringVar(&C.Ebpfgo.DstSpec, "e3dst", "", "目标地址 CIDR 过滤")
	flag.UintVar(&C.Ebpfgo.Sport, "e3sport", 0, "源端口过滤值")
	flag.UintVar(&C.Ebpfgo.Dport, "e3dport", 0, "目标端口过滤值")
	flag.Int64Var(&C.Ebpfgo.MaxBodySize, "e3maxbodysize", -1, "HTTP body 保留上限，默认 -1")

	z.Register("14-ebpfgo", func(zgg *z.Zgg) z.Closed {
		cfg := normalizeInitConfig(C.Ebpfgo)
		if cfg.Disabled {
			z.Logn("[_ebpfgo_]: disabled")
			return nil
		}
		if _, err := normalizeConfig(cfg); err != nil {
			zgg.ServeStop("register ebpfgo error by config,", err.Error())
			return nil
		}
		srv, err := NewServer(cfg, nil)
		if err != nil {
			zgg.ServeStop("register ebpfgo error by server,", err.Error())
			return nil
		}
		zgg.Servers.Add(srv)
		if ifn != nil {
			ifn(srv, zgg)
		}
		z.Logn("[_ebpfgo_]: registered", fmt.Sprintf("f=%s dir=%s", //
			zc.If(cfg.IfName != "", cfg.IfName, "all"), //
			zc.If(cfg.Direction != "", cfg.Direction, "both")))
		return nil
	})
}

func normalizeInitConfig(cfg Config) Config {
	cfg.IfName = strings.TrimSpace(cfg.IfName)
	cfg.PcapRules = strings.TrimSpace(cfg.PcapRules)
	cfg.Direction = strings.TrimSpace(cfg.Direction)
	cfg.Comm = strings.TrimSpace(cfg.Comm)
	cfg.SrcSpec = strings.TrimSpace(cfg.SrcSpec)
	cfg.DstSpec = strings.TrimSpace(cfg.DstSpec)
	return cfg
}

type Hook func(Event)

type Event struct {
	Packet PacketEvent
	Meta   *SocketMeta
	Record map[string]any
}

type Server struct {
	cfg    runtimeConfig
	hook   Hook
	ctx    context.Context
	cancel context.CancelFunc

	mu      sync.Mutex
	started bool
	closed  bool
	errExit bool

	rawSock int
	objs    bpfObjects
	links   []link.Link
	state   monitorState
}

func NewServer(cfg Config, hook Hook) (z.Server, error) {
	rc, err := normalizeConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("invalid config: %v\n", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Server{
		cfg:     rc,
		hook:    hook,
		ctx:     ctx,
		cancel:  cancel,
		rawSock: -1,
		state: monitorState{
			flows:   make(map[flowKey]*flowState),
			sockets: socketCache{items: make(map[socketKey]SocketMeta)},
		},
		errExit: true,
	}, nil
}

func (s *Server) Name() string {
	return "(EBPFG)"
}

func (s *Server) Addr() string {
	if s.cfg.IfName != "" {
		return s.cfg.IfName
	}
	return "all interfaces"
}

func (s *Server) RunServe() {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return
	}
	s.started = true
	s.mu.Unlock()

	if err := s.run(); err != nil && !errors.Is(err, context.Canceled) {
		z.Exit(fmt.Sprintf("[_ebpfgo_]: server exit error: %v\n", err))
	}
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.cancel()
	if s.rawSock >= 0 {
		_ = syscall.Close(s.rawSock)
		s.rawSock = -1
	}
	links := s.links
	s.links = nil
	objs := s.objs
	s.mu.Unlock()

	for _, l := range links {
		_ = l.Close()
	}
	_ = objs.Close()

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}
