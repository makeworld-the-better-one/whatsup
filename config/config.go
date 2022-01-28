package config

type ServerConf struct {
	Host string
	Port uint16
	Cert string
	Key  string
}

type DataConf struct {
	Dir string
}

type TomlConfig struct {
	Server ServerConf
	Data   DataConf
	Users  map[string]string
}

var Conf TomlConfig
