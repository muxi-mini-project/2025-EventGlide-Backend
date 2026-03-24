package ioc

import (
	"github.com/IBM/sarama"
	"github.com/raiki02/EG/config"
)

type Kafka struct {
	P sarama.SyncProducer
	C sarama.Consumer
}

func NewKafkaClient(cfg *config.Conf) *sarama.Client {
	addr := []string{cfg.Kafka.Addr}
	config := sarama.NewConfig()
	config.Producer.RequiredAcks = sarama.WaitForAll
	config.Producer.Partitioner = sarama.NewRandomPartitioner
	config.Producer.Return.Successes = true
	client, err := sarama.NewClient(addr, config)
	if err != nil {
		panic(err)
	}
	return &client
}

func NewProducer(client *sarama.Client) sarama.SyncProducer {
	producer, err := sarama.NewSyncProducerFromClient(*client)
	if err != nil {
		panic(err)
	}
	return producer
}

func NewConsumer(client *sarama.Client) sarama.Consumer {
	consumer, err := sarama.NewConsumerFromClient(*client)
	if err != nil {
		panic(err)
	}
	return consumer
}

func NewKafka(p sarama.SyncProducer, c sarama.Consumer) *Kafka {
	return &Kafka{
		P: p,
		C: c,
	}
}
