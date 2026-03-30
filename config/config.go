package config

import (
	"bytes"
	"fmt"
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"

	"github.com/nacos-group/nacos-sdk-go/clients"
	"github.com/nacos-group/nacos-sdk-go/common/constant"
	"github.com/nacos-group/nacos-sdk-go/vo"
	"github.com/spf13/viper"
)

type Conf struct {
	Mysql   MysqlConf   `yaml:"mysql"`
	Redis   RedisConf   `yaml:"redis"`
	JWT     JwtConf     `yaml:"jwt"`
	Imgbed  ImgbedConf  `yaml:"imgbed"`
	Auditor AuditorConf `yaml:"auditor"`
	Kafka   KafkaConf   `yaml:"kafka"`
}

type MysqlConf struct {
	DSN          string `yaml:"dsn"`
	MaxIdleConns int    `yaml:"maxIdleConns"`
	MaxOpenConns int    `yaml:"maxOpenConns"`
}

type RedisConf struct {
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
}

type JwtConf struct {
	Key string `yaml:"key"`
	Ttl int    `yaml:"ttl"`
}

type ImgbedConf struct {
	AccessKey      string `yaml:"accessKey"`
	SecretKey      string `yaml:"secretKey"`
	Bucket         string `yaml:"bucket"`
	ImgURL         string `yaml:"imgUrl"`
	DefaultAvatar1 string `yaml:"defaultAvatar1"`
}

type AuditorConf struct {
	Region      string `yaml:"region"`
	HookURL     string `yaml:"hookUrl"`
	ApiKey      string `yaml:"apiKey"`
	WebHookPath string `yaml:"webhookPath"`
	Effect      string `yaml:"effect"`
}

type KafkaConf struct {
	Addr string `yaml:"addr"`
}

const (
	EgConf           = "EVENTGLIDE_NACOS_CONF"
	LocalConfPathEnv = "EVENTGLIDE_LOCAL_CONF"
)

func InitConf() *Conf {
	var _ = godotenv.Load()
	content, err := getConfigFromNacos(EgConf)
	if err != nil {
		log.Printf("Nacos 配置读取失败，尝试本地配置: %v", err)
		content, err = readLocalConfig()
		if err != nil {
			log.Fatalf("无法读取本地配置，且 Nacos 配置获取失败: %v", err)
			return nil
		}
	}

	v := viper.New()
	v.SetConfigType("yaml")

	if err = v.ReadConfig(bytes.NewBufferString(content)); err != nil {
		log.Fatal("配置文件解析失败:", err)
		return nil
	}

	var eg Conf
	err = v.Unmarshal(&eg)
	if err != nil {
		log.Fatal(err)
		return nil
	}

	return &eg
}

func getConfigFromNacos(env string) (string, error) {
	server, port, namespace, user, pass, group, dataId, err := parseNacosDSN(env)
	if err != nil {
		return "", err
	}

	if group == "" {
		group = "DEFAULT_GROUP"
	}
	if dataId == "" {
		return "", fmt.Errorf("nacos dataId is empty in %s", env)
	}

	serverConfigs := []constant.ServerConfig{
		{
			IpAddr: server,
			Port:   port,
			Scheme: "http",
		},
	}

	clientConfig := constant.ClientConfig{
		NamespaceId:         namespace,
		Username:            user,
		Password:            pass,
		TimeoutMs:           5000,
		NotLoadCacheAtStart: true,
		CacheDir:            "./data/configCache",
	}

	configClient, err := clients.CreateConfigClient(map[string]interface{}{
		"serverConfigs": serverConfigs,
		"clientConfig":  clientConfig,
	})
	if err != nil {
		return "", fmt.Errorf("nacos client init failed: %w", err)
	}

	content, err := configClient.GetConfig(vo.ConfigParam{
		DataId: dataId,
		Group:  group,
	})
	if err != nil {
		return "", fmt.Errorf("nacos get config failed: %w", err)
	}

	if strings.TrimSpace(content) == "" {
		return "", fmt.Errorf("nacos returned empty config content")
	}
	return content, nil
}

func parseNacosDSN(env string) (server string, port uint64, ns, user, pass, group, dataId string, err error) {
	dsn := os.Getenv(env)
	if dsn == "" {
		err = fmt.Errorf("%s 环境变量未设置", env)
		return
	}

	parts := strings.SplitN(dsn, "?", 2)
	host := parts[0]
	if host == "" {
		err = fmt.Errorf("%s 环境变量格式错误：缺少 host", env)
		return
	}
	params := url.Values{}

	if len(parts) == 2 {
		params, _ = url.ParseQuery(parts[1])
	}

	hostParts := strings.Split(host, ":")
	server = hostParts[0]
	if server == "" {
		err = fmt.Errorf("%s 环境变量格式错误：缺少 server", env)
		return
	}
	if len(hostParts) > 1 {
		p, convErr := strconv.Atoi(hostParts[1])
		if convErr != nil || p <= 0 {
			err = fmt.Errorf("%s 环境变量端口非法: %s", env, hostParts[1])
			return
		}
		port = uint64(p)
	} else {
		port = 8848
	}

	ns = params.Get("namespace")
	if ns == "" {
		ns = "public"
	}

	user = params.Get("username")
	pass = params.Get("password")
	group = params.Get("group")
	dataId = params.Get("dataId")
	return
}

func readLocalConfig() (string, error) {
	candidates := make([]string, 0, 4)
	if p := strings.TrimSpace(os.Getenv(LocalConfPathEnv)); p != "" {
		candidates = append(candidates, p)
	}
	candidates = append(candidates,
		"./config/conf.yaml",
		"./config/conf1.yaml",
		"./config/conf-example.yaml",
	)

	var errs []string
	for _, path := range candidates {
		content, err := os.ReadFile(path)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", path, err))
			continue
		}
		if strings.TrimSpace(string(content)) == "" {
			errs = append(errs, fmt.Sprintf("%s: empty file", path))
			continue
		}
		log.Printf("使用本地配置文件: %s", path)
		return string(content), nil
	}

	return "", fmt.Errorf("all local config candidates failed: %s", strings.Join(errs, "; "))
}
