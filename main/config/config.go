package config

import (
	"crypto/tls"
	"fmt"
	"github.com/danieldin95/openlan-go/libol"
	"github.com/xtaci/kcp-go/v5"
	"os"
	"strings"
)

type Log struct {
	File    string `json:"file,omitempty" yaml:"file,omitempty"`
	Verbose int    `json:"level,omitempty" yaml:"level,omitempty"`
}

type Http struct {
	Listen string `json:"listen,omitempty" yaml:"listen,omitempty"`
	Public string `json:"public,omitempty" yaml:"public,omitempty"`
}

type Crypt struct {
	Algo   string `json:"lago,omitempty" yaml:"algo,omitempty"`
	Secret string `json:"secret,omitempty" yaml:"secret,omitempty"`
}

func CryptDefault(cfg *Crypt) {
	if cfg == nil {
		return
	}
	if cfg.Algo == "" {
		cfg.Algo = "xor"
	}
	if cfg.Secret == "" {
		cfg.Secret = "3ir456dew"
	}
}

func RightAddr(listen *string, port int) {
	values := strings.Split(*listen, ":")
	if len(values) == 1 {
		*listen = fmt.Sprintf("%s:%d", values[0], port)
	}
}

func GetAlias() string {
	if hostname, err := os.Hostname(); err == nil {
		return hostname
	}
	return libol.GenToken(13)
}

func GetTlsCfg(cfg Cert) *tls.Config {
	if cfg.KeyFile != "" && cfg.CrtFile != "" {
		cer, err := tls.LoadX509KeyPair(cfg.CrtFile, cfg.KeyFile)
		if err != nil {
			libol.Error("NewSwitch: %s", err)
		}
		return &tls.Config{Certificates: []tls.Certificate{cer}}
	}
	return nil
}

func GetBlock(cfg Crypt) kcp.BlockCrypt {
	var block kcp.BlockCrypt
	pass := make([]byte, 64)
	if len(cfg.Secret) <= 64 {
		copy(pass, cfg.Secret)
	} else {
		copy(pass, []byte(cfg.Secret)[:64])
	}
	switch cfg.Algo {
	case "aes-128":
		block, _ = kcp.NewAESBlockCrypt(pass[:16])
	case "aes-192":
		block, _ = kcp.NewAESBlockCrypt(pass[:24])
	default:
		block, _ = kcp.NewSimpleXORBlockCrypt(pass)
	}
	return block
}
