package config

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Interface  string        `yaml:"interface"`
	DataDir    string        `yaml:"-"`
	Web        WebConfig     `yaml:"web"`
	Collectors CollectorsCfg `yaml:"collectors"`
	AI         AICfg         `yaml:"ai"`
	Alerts     AlertsCfg     `yaml:"alerts"`
	Storage    StorageCfg    `yaml:"storage"`
}

type StorageCfg struct {
	LogRetentionDays int `yaml:"log_retention_days"`
}

type WebConfig struct {
	Listen string   `yaml:"listen"`
	Auth   AuthCfg  `yaml:"auth"`
}

type AuthCfg struct {
	Enabled  bool   `yaml:"enabled"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type CollectorsCfg struct {
	ARP  ARPCfg  `yaml:"arp"`
	MDNS MDNSCfg `yaml:"mdns"`
	DHCP DHCPCfg `yaml:"dhcp"`
	Ping PingCfg `yaml:"ping"`
	Nmap NmapCfg `yaml:"nmap"`
}

type ARPCfg struct {
	Enabled        bool          `yaml:"enabled"`
	OfflineTimeout time.Duration `yaml:"offline_timeout"`
}

type MDNSCfg struct {
	Enabled bool `yaml:"enabled"`
}

type DHCPCfg struct {
	Enabled bool `yaml:"enabled"`
}

type PingCfg struct {
	Enabled  bool          `yaml:"enabled"`
	Targets  string        `yaml:"targets"`
	Interval time.Duration `yaml:"interval"`
	Timeout  time.Duration `yaml:"timeout"`
}

type NmapCfg struct {
	Enabled bool   `yaml:"enabled"`
	Trigger string `yaml:"trigger"`
	Args    string `yaml:"args"`
}

type AICfg struct {
	Fingerprint FingerprintCfg `yaml:"fingerprint"`
	Correlation CorrelationCfg `yaml:"correlation"`
	Anomaly     AnomalyCfg     `yaml:"anomaly"`
}

type FingerprintCfg struct {
	Enabled bool `yaml:"enabled"`
}

type CorrelationCfg struct {
	Enabled    bool          `yaml:"enabled"`
	OllamaURL  string        `yaml:"ollama_url"`
	Model      string        `yaml:"model"`
	Window     time.Duration `yaml:"window"`
}

type AnomalyCfg struct {
	Enabled          bool `yaml:"enabled"`
	BaselineDays     int  `yaml:"baseline_days"`
	MinObservations  int  `yaml:"min_observations"`
}

type AlertsCfg struct {
	Rules   AlertRules   `yaml:"rules"`
	Channels ChannelsCfg `yaml:"channels"`
	Routing RoutingCfg   `yaml:"routing"`
}

type AlertRules struct {
	NewDevice       RuleCfg `yaml:"new_device"`
	PriorityOffline RuleCfg `yaml:"priority_offline"`
	DeviceBack      RuleCfg `yaml:"device_back"`
	ServiceDown     RuleCfg `yaml:"service_down"`
	ServiceBack     RuleCfg `yaml:"service_back"`
	Anomaly         RuleCfg `yaml:"anomaly"`
}

type RuleCfg struct {
	Enabled   bool          `yaml:"enabled"`
	Severity  string        `yaml:"severity"`
	Threshold time.Duration `yaml:"threshold"`
	Cooldown  time.Duration `yaml:"cooldown"`
	MinScore  float64       `yaml:"min_score"`
}

type ChannelsCfg struct {
	Ntfy     NtfyCfg     `yaml:"ntfy"`
	Discord  DiscordCfg  `yaml:"discord"`
	Webhook  WebhookCfg  `yaml:"webhook"`
	Email    EmailCfg    `yaml:"email"`
}

type DiscordCfg struct {
	Enabled    bool   `yaml:"enabled"`
	WebhookURL string `yaml:"webhook_url"`
}

type NtfyCfg struct {
	Enabled bool   `yaml:"enabled"`
	URL     string `yaml:"url"`
	Token   string `yaml:"token"`
}

type WebhookCfg struct {
	Enabled bool              `yaml:"enabled"`
	URL     string            `yaml:"url"`
	Method  string            `yaml:"method"`
	Headers map[string]string `yaml:"headers"`
}

type EmailCfg struct {
	Enabled  bool     `yaml:"enabled"`
	SMTPHost string   `yaml:"smtp_host"`
	SMTPPort int      `yaml:"smtp_port"`
	SMTPUser string   `yaml:"smtp_user"`
	SMTPPass string   `yaml:"smtp_pass"`
	From     string   `yaml:"from"`
	To       []string `yaml:"to"`
}

type RoutingCfg struct {
	Critical []string `yaml:"critical"`
	High     []string `yaml:"high"`
	Medium   []string `yaml:"medium"`
	Info     []string `yaml:"info"`
	Low      []string `yaml:"low"`
}

func Defaults() *Config {
	return &Config{
		Interface: "",
		DataDir:   "/data",
		Web: WebConfig{
			Listen: "0.0.0.0:8080",
		},
		Collectors: CollectorsCfg{
			ARP:  ARPCfg{Enabled: true, OfflineTimeout: 15 * time.Minute},
			MDNS: MDNSCfg{Enabled: true},
			DHCP: DHCPCfg{Enabled: true},
			Ping: PingCfg{Enabled: true, Targets: "priority", Interval: 5 * time.Minute, Timeout: 3 * time.Second},
			Nmap: NmapCfg{Enabled: false, Trigger: "new_device", Args: "-sV --top-ports 20 -T3"},
		},
		AI: AICfg{
			Fingerprint: FingerprintCfg{Enabled: true},
			Correlation: CorrelationCfg{Enabled: false, OllamaURL: "http://localhost:11434", Model: "phi3:mini", Window: 90 * time.Second},
			Anomaly:     AnomalyCfg{Enabled: true, BaselineDays: 14, MinObservations: 50},
		},
		Storage: StorageCfg{LogRetentionDays: 30},
		Alerts: AlertsCfg{
			Rules: AlertRules{
				NewDevice:       RuleCfg{Enabled: true, Severity: "high"},
				PriorityOffline: RuleCfg{Enabled: true, Severity: "critical", Threshold: 10 * time.Minute, Cooldown: 30 * time.Minute},
				DeviceBack:      RuleCfg{Enabled: true, Severity: "high", Cooldown: 5 * time.Minute},
				ServiceDown:     RuleCfg{Enabled: true, Severity: "high", Cooldown: 15 * time.Minute},
				ServiceBack:     RuleCfg{Enabled: true, Severity: "high", Cooldown: 5 * time.Minute},
				Anomaly:         RuleCfg{Enabled: true, Severity: "medium", MinScore: 0.7, Cooldown: 60 * time.Minute},
			},
			Routing: RoutingCfg{
				Critical: []string{"ntfy", "discord", "email"},
				High:     []string{"ntfy", "discord"},
				Medium:   []string{"webhook"},
				Info:     []string{},
				Low:      []string{},
			},
		},
	}
}

func Load(path string) (*Config, error) {
	cfg := Defaults()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}
