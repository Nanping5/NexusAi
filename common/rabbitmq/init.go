package rabbitmq

import (
	"context"
	mylogger "NexusAi/pkg/logger"
)

var (
	RMQMessage *RabbitMQ
)

// InitRabbitMQ 初始化 RabbitMQ 连接和消费者
func InitRabbitMQ() {
	InitConn()
	RMQMessage = NewWorkerRabbitMQ("Message")

	// 启动带 context 控制的消费者
	ctx, cancel := context.WithCancel(context.Background())
	consumerCancel = cancel
	consumerWg.Add(1)
	go func() {
		defer consumerWg.Done()
		// 添加 recover 防止 panic 导致整个程序崩溃
		defer func() {
			if r := recover(); r != nil {
				mylogger.Logger.Error("RabbitMQ consumer panic recovered: " + toString(r))
			}
		}()
		RMQMessage.ConsumeContext(ctx, MQMessage)
	}()

	// 启动连接监控
	StartConnectionMonitor()
}

// DestroyRabbitMQ 关闭 RabbitMQ 连接和通道
func DestroyRabbitMQ() {
	// 先停止消费者
	if consumerCancel != nil {
		consumerCancel()
		consumerWg.Wait()
	}

	if RMQMessage != nil {
		RMQMessage.Destroy()
	}
	CloseGlobalConn()
}

func toString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return "unknown error"
}
