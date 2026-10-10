// Package config 负责加载和校验启动配置。
// 运行时可修改的开关不在这里，见 service.SettingService。
package config

import (
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Bot     Bot     `mapstructure:"bot"`
	Web     Web     `mapstructure:"web"`
	Captcha Captcha `mapstructure:"captcha"`
	DB      DB      `mapstructure:"db"`
}

type Bot struct {
	Token        string  `mapstructure:"token"`
	Mode         string  `mapstructure:"mode"` // polling | webhook
	AdminGroupID int64   `mapstructure:"admin_group_id"`
	AdminIDs     []int64 `mapstructure:"admin_ids"`
}

type Web struct {
	Listen    string `mapstructure:"listen"`
	PublicURL string `mapstructure:"public_url"`
}

type Captcha struct {
	Turnstile Turnstile `mapstructure:"turnstile"`
	Cap       Cap       `mapstructure:"cap"`
}

type Turnstile struct {
	SiteKey string `mapstructure:"site_key"`
	Secret  string `mapstructure:"secret"`
}

type Cap struct {
	URL     string `mapstructure:"url"`
	SiteKey string `mapstructure:"site_key"`
	Secret  string `mapstructure:"secret"`
}

type DB struct {
	Path string `mapstructure:"path"`
}

const (
	ModePolling = "polling"
	ModeWebhook = "webhook"
)

// Load 读取配置文件，环境变量（前缀 DBMTG_，如 DBMTG_BOT_TOKEN）可覆盖文件中的值
func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetDefault("bot.mode", ModePolling)
	v.SetDefault("web.listen", ":8080")
	v.SetDefault("db.path", "data/dbmtg.db")
	v.SetEnvPrefix("DBMTG")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}
	var c Config
	if err := v.Unmarshal(&c); err != nil {
		return nil, err
	}
	c.Web.PublicURL = strings.TrimRight(c.Web.PublicURL, "/")
	c.Captcha.Cap.URL = strings.TrimRight(c.Captcha.Cap.URL, "/")
	return &c, c.validate()
}

func (c *Config) validate() error {
	switch {
	case c.Bot.Token == "":
		return fmt.Errorf("bot.token 未配置")
	case c.Bot.AdminGroupID == 0:
		return fmt.Errorf("bot.admin_group_id 未配置")
	case len(c.Bot.AdminIDs) == 0:
		return fmt.Errorf("bot.admin_ids 未配置")
	case c.Bot.Mode != ModePolling && c.Bot.Mode != ModeWebhook:
		return fmt.Errorf("bot.mode 只能是 polling 或 webhook")
	case c.Bot.Mode == ModeWebhook && c.Web.PublicURL == "":
		return fmt.Errorf("webhook 模式需要配置 web.public_url")
	}
	return nil
}

func (c *Config) IsAdmin(id int64) bool { return slices.Contains(c.Bot.AdminIDs, id) }
