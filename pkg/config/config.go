package config

// TcpServerConfigs TCP Server configurations
type TcpServerConfigs struct {
	Type string               `koanf:"type"`
	Host string               `koanf:"host"`
	Port string               `koanf:"port"`
	Pool TCPServerPoolConfigs `koanf:"conn-pool"`
}

// TCPServerPoolConfigs TCP Server configurations
type TCPServerPoolConfigs struct {
	MaxSize            int `koanf:"max-size"`
	WaitForConnTimeout int `koanf:"wait-for-conn-timeout-ms"`
	NoOpCycle          int `koanf:"noop-cycle-ms"`
}
