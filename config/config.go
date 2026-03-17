package config

import (
	"bytes"
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"

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
	EgConf = "EVENTGLIDE_NACOS_CONF"
)

func InitConf() *Conf {
	content, err := getConfigFromNacos(EgConf)
	if err != nil {
		log.Println(err)

		localPath := "./conf.yaml"
		fileContent, err := os.ReadFile(localPath)
		if err != nil {
			// 如果本地文件也读取失败，则彻底失败
			log.Fatalf("无法读取本地配置文件 %s，且 Nacos 配置获取失败: %v", localPath, err)
			return nil
		}
		content = string(fileContent)
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
	server, port, namespace, user, pass, group, dataId := parseNacosDSN(env)

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
		log.Fatal("初始化失败:", err)
	}

	content, err := configClient.GetConfig(vo.ConfigParam{
		DataId: dataId,
		Group:  group,
	})
	if err != nil {
		log.Fatal("拉取配置失败:", err)
	}
	return content, nil
}

func parseNacosDSN(env string) (server string, port uint64, ns, user, pass, group, dataId string) {
	dsn := os.Getenv(env)
	if dsn == "" {
		log.Fatalf("%s 环境变量未设置", env)
	}

	parts := strings.SplitN(dsn, "?", 2)
	host := parts[0]
	params := url.Values{}

	if len(parts) == 2 {
		params, _ = url.ParseQuery(parts[1])
	}

	hostParts := strings.Split(host, ":")
	server = hostParts[0]
	if len(hostParts) > 1 {
		p, _ := strconv.Atoi(hostParts[1])
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
