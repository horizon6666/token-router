package conf

import (
	"flag"
	"fmt"
	"time"
)

type Config struct {
	Nodes  int
	Budget int64
	Addr   string

	HTTPReadHeaderTimeout time.Duration
	ShutdownTimeout       time.Duration
}

var GlobalConfigs Config

func Init() error {
	n := flag.Int("n", 2, "number of nodes")
	m := flag.Int64("m", 300, "per-node token budget")
	addr := flag.String("addr", ":8080", "listen address")
	flag.Parse()

	if *n <= 0 || *m <= 0 {
		return fmt.Errorf("invalid config: n=%d m=%d (both must be > 0)", *n, *m)
	}

	GlobalConfigs = Config{
		Nodes:                 *n,
		Budget:                *m,
		Addr:                  *addr,
		HTTPReadHeaderTimeout: 5 * time.Second,
		ShutdownTimeout:       5 * time.Second,
	}
	return nil
}
