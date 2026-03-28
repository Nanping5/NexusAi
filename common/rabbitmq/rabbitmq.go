package rabbitmq

import (
	"NexusAi/config"
	mylogger "NexusAi/pkg/logger"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/streadway/amqp"
)

// globalConn 全局 RabbitMQ 连接
var globalConn *amqp.Connection
var connMutex sync.RWMutex

// 连接状态
var isConnected bool

// 消费者相关
var consumerCancel context.CancelFunc
var consumerWg sync.WaitGroup

type RabbitMQ struct {
	conn     *amqp.Connection
	channel  *amqp.Channel
	Exchange string
	Key      string
}

// InitConn 初始化 RabbitMQ 连接
func InitConn() {
	connMutex.Lock()
	defer connMutex.Unlock()

	if globalConn != nil && !globalConn.IsClosed() {
		return
	}

	c := config.GetConfig()
	mqURL := fmt.Sprintf("amqp://%s:%s@%s:%d/%s",
		c.RabbitMQConfig.Username, c.RabbitMQConfig.Password, c.RabbitMQConfig.Host, c.RabbitMQConfig.Port, c.RabbitMQConfig.Vhost)
	mylogger.Logger.Info("RabbitMQ connection initialized")

	var err error
	globalConn, err = amqp.Dial(mqURL)

	if err != nil {
		mylogger.Logger.Error("Failed to connect to RabbitMQ: " + err.Error())
		panic(err)
	}

	isConnected = true
}

// StartConnectionMonitor 启动连接监控协程
func StartConnectionMonitor() {
	go monitorConnection()
}

// monitorConnection 监控连接状态并自动重连
func monitorConnection() {
	for {
		time.Sleep(5 * time.Second)
		connMutex.RLock()
		shouldReconnect := globalConn == nil || globalConn.IsClosed()
		connMutex.RUnlock()

		if shouldReconnect {
			connMutex.Lock()
			isConnected = false
			mylogger.Logger.Warn("RabbitMQ connection lost, attempting to reconnect...")
			connMutex.Unlock()
			reconnectAndRestartConsumer()
		}
	}
}

// reconnectAndRestartConsumer 重连并重启消费者
func reconnectAndRestartConsumer() {
	connMutex.Lock()
	defer connMutex.Unlock()

	c := config.GetConfig()
	mqURL := fmt.Sprintf("amqp://%s:%s@%s:%d/%s",
		c.RabbitMQConfig.Username, c.RabbitMQConfig.Password, c.RabbitMQConfig.Host, c.RabbitMQConfig.Port, c.RabbitMQConfig.Vhost)

	var err error
	for i := 0; i < 5; i++ {
		globalConn, err = amqp.Dial(mqURL)
		if err == nil {
			isConnected = true
			mylogger.Logger.Info("RabbitMQ reconnected successfully")
			// 重连成功后重启消费者
			restartConsumer()
			return
		}
		mylogger.Logger.Error(fmt.Sprintf("RabbitMQ reconnect attempt %d failed: %s", i+1, err.Error()))
		time.Sleep(time.Duration(i+1) * time.Second)
	}
	mylogger.Logger.Error("Failed to reconnect to RabbitMQ after 5 attempts")
}

// restartConsumer 重启消息消费者
func restartConsumer() {
	// 取消旧的消费者
	if consumerCancel != nil {
		consumerCancel()
		consumerWg.Wait()
	}

	// 重新创建 channel 和消费者
	if RMQMessage != nil && globalConn != nil {
		var err error
		RMQMessage.channel, err = globalConn.Channel()
		if err != nil {
			mylogger.Logger.Error("Failed to create new channel after reconnect: " + err.Error())
			return
		}

		ctx, cancel := context.WithCancel(context.Background())
		consumerCancel = cancel
		consumerWg.Add(1)
		go func() {
			defer consumerWg.Done()
			RMQMessage.ConsumeContext(ctx, MQMessage)
		}()
		mylogger.Logger.Info("RabbitMQ consumer restarted")
	}
}

// CloseGlobalConn 关闭全局 RabbitMQ 连接
func CloseGlobalConn() {
	// 先停止消费者
	if consumerCancel != nil {
		consumerCancel()
		consumerWg.Wait()
	}

	connMutex.Lock()
	defer connMutex.Unlock()

	if globalConn != nil {
		if err := globalConn.Close(); err != nil {
			mylogger.Logger.Error("Failed to close global RabbitMQ connection: " + err.Error())
		}
	}
}

// NewRabbitMQ 创建一个新的 RabbitMQ 实例
func NewRabbitMQ(exchange, key string) *RabbitMQ {
	return &RabbitMQ{
		Exchange: exchange,
		Key:      key,
	}
}

// Destroy 关闭 RabbitMQ 连接和通道
func (r *RabbitMQ) Destroy() {
	if r.channel != nil {
		if err := r.channel.Close(); err != nil {
			mylogger.Logger.Error("Failed to close RabbitMQ channel: " + err.Error())
		}
	}
	if r.conn != nil {
		if err := r.conn.Close(); err != nil {
			mylogger.Logger.Error("Failed to close RabbitMQ connection: " + err.Error())
		}
	}
}

// NewWorkerRabbitMQ 创建一个新的 RabbitMQ 实例用于工作队列
func NewWorkerRabbitMQ(queue string) *RabbitMQ {
	rabbitmq := NewRabbitMQ("", queue)

	if globalConn == nil {
		InitConn()
	}
	rabbitmq.conn = globalConn

	var err error
	rabbitmq.channel, err = rabbitmq.conn.Channel()
	if err != nil {
		mylogger.Logger.Error("Failed to open a channel: " + err.Error())
		panic(err)
	}

	return rabbitmq
}

// Publish 向 RabbitMQ 发布消息
func (r *RabbitMQ) Publish(message []byte) error {

	_, err := r.channel.QueueDeclare(
		r.Key,
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		mylogger.Logger.Error("Failed to declare a queue: " + err.Error())
		return err
	}
	return r.channel.Publish(
		r.Exchange,
		r.Key,
		false,
		false,
		amqp.Publishing{
			ContentType: "text/plain",
			Body:        message,
		},
	)
}

// ConsumeContext 支持上下文取消的消息消费
func (r *RabbitMQ) ConsumeContext(ctx context.Context, handle func(msg *amqp.Delivery) error) error {
	q, err := r.channel.QueueDeclare(
		r.Key,
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		mylogger.Logger.Error("Failed to declare a queue: " + err.Error())
		return err
	}

	// 关闭自动确认，改为手动确认
	msgs, err := r.channel.Consume(
		q.Name,
		"",
		false, // autoAck: false，手动确认
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		mylogger.Logger.Error("Failed to register a consumer: " + err.Error())
		return err
	}

	for {
		select {
		case <-ctx.Done():
			mylogger.Logger.Info("RabbitMQ consumer shutting down...")
			return ctx.Err()
		case msg, ok := <-msgs:
			if !ok {
				mylogger.Logger.Warn("RabbitMQ message channel closed")
				return nil
			}
			err := handle(&msg)
			if err != nil {
				mylogger.Logger.Error("Failed to handle message: " + err.Error())
				// 处理失败，Nack 消息，不重新入队
				if nackErr := msg.Nack(false, false); nackErr != nil {
					mylogger.Logger.Error("Failed to Nack message: " + nackErr.Error())
				}
			} else {
				// 处理成功，Ack 消息
				if ackErr := msg.Ack(false); ackErr != nil {
					mylogger.Logger.Error("Failed to Ack message: " + ackErr.Error())
				}
			}
		}
	}
}

// Consume 从 RabbitMQ 中消费消息，并使用提供的处理函数处理每条消息
func (r *RabbitMQ) Consume(handle func(msg *amqp.Delivery) error) {

	q, err := r.channel.QueueDeclare(
		r.Key,
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		mylogger.Logger.Error("Failed to declare a queue: " + err.Error())
		return
	}

	// 关闭自动确认，改为手动确认
	msgs, err := r.channel.Consume(
		q.Name,
		"",
		false, // autoAck: false，手动确认
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		mylogger.Logger.Error("Failed to register a consumer: " + err.Error())
		return
	}

	for msg := range msgs {
		err := handle(&msg)
		if err != nil {
			mylogger.Logger.Error("Failed to handle message: " + err.Error())
			// 处理失败，Nack 消息，不重新入队
			if nackErr := msg.Nack(false, false); nackErr != nil {
				mylogger.Logger.Error("Failed to Nack message: " + nackErr.Error())
			}
		} else {
			// 处理成功，Ack 消息
			if ackErr := msg.Ack(false); ackErr != nil {
				mylogger.Logger.Error("Failed to Ack message: " + ackErr.Error())
			}
		}
	}
}
