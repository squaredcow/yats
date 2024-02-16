package config

// TcpServerConfigs represents the configuration parameters necessary
// for establishing a TCP server connection.
// These configurations are https://github.com/knadh/koanf compatible.
type TcpServerConfigs struct {
	Type     string             `koanf:"type"`
	Host     string             `koanf:"host"`
	Port     string             `koanf:"port"`
	ConnPool TcpConnPoolConfigs `koanf:"conn-pool"`
}

// TcpConnPoolConfigs represent configuration parameters for managing
// a pool of TCP connections.
// These configurations are https://github.com/knadh/koanf compatible.
type TcpConnPoolConfigs struct {
	MaxSize            int `koanf:"max-size"`
	WaitForConnTimeout int `koanf:"wait-for-conn-timeout-ms"`
	NoOpCycle          int `koanf:"noop-cycle-ms"`
}
