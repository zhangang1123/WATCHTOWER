package agent

import (
	"context"
	"fmt"
	"io"
	"time"

	"watchtower-lite/internal/models"
	pb "watchtower-lite/proto"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Client gRPC 客户端
type Client struct {
	conn   *grpc.ClientConn
	client pb.DiagnosisServiceClient
	addr   string
	logger *zap.Logger
}

// NewClient 创建 Agent 客户端
func NewClient(addr string, logger *zap.Logger) (*Client, error) {
	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to agent: %w", err)
	}

	return &Client{
		conn:   conn,
		client: pb.NewDiagnosisServiceClient(conn),
		addr:   addr,
		logger: logger,
	}, nil
}

// Diagnose 触发 Agent 诊断
func (c *Client) Diagnose(ctx context.Context, incident *models.Incident) (*models.Diagnosis, error) {
	req := &pb.DiagnoseRequest{
		IncidentId:       incident.ID,
		Service:          incident.Service,
		AlertDescription: incident.Description,
		Severity:         string(incident.Severity),
		AlertTimestamp:   incident.CreatedAt.Unix(),
	}

	resp, err := c.client.Diagnose(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("diagnosis failed: %w", err)
	}

	return convertFromProto(resp), nil
}

// DiagnoseStream 流式诊断（实时获取每一步推理过程）
func (c *Client) DiagnoseStream(ctx context.Context, incident *models.Incident, onStep func(*models.DiagnosisStep)) (*models.Diagnosis, error) {
	req := &pb.DiagnoseRequest{
		IncidentId:       incident.ID,
		Service:          incident.Service,
		AlertDescription: incident.Description,
		Severity:         string(incident.Severity),
		AlertTimestamp:   incident.CreatedAt.Unix(),
	}

	stream, err := c.client.DiagnoseStream(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("stream diagnosis failed: %w", err)
	}

	var finalDiagnosis *models.Diagnosis

	for {
		step, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("stream recv error: %w", err)
		}

		if step.IsFinal {
			// 最后一步是完整诊断结果
			finalDiagnosis = convertFromProto(step.FinalResult)
			break
		}

		ds := &models.DiagnosisStep{
			StepNumber:  step.StepNumber,
			Thought:     step.Thought,
			Action:      step.Action,
			Observation: step.Observation,
			LatencyMs:   step.LatencyMs,
		}
		onStep(ds)
	}

	return finalDiagnosis, nil
}

// HealthCheck 健康检查
func (c *Client) HealthCheck(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	_, err := c.client.HealthCheck(ctx, &pb.HealthRequest{})
	return err
}

// Close 关闭连接
func (c *Client) Close() error {
	return c.conn.Close()
}

// convertFromProto 将 protobuf 转为内部模型
func convertFromProto(resp *pb.DiagnoseResponse) *models.Diagnosis {
	d := &models.Diagnosis{
		IncidentID:   resp.IncidentId,
		RootCause:    resp.RootCause,
		Confidence:   float64(resp.Confidence),
		SuggestedFix: resp.SuggestedFix,
		FixType:      resp.FixType,
		AutoFixable:  resp.AutoFixable,
		Escalated:    resp.Escalated,
		Summary:      resp.Summary,
	}

	for _, e := range resp.Evidence {
		d.Evidence = append(d.Evidence, models.Evidence{
			Source:      e.Source,
			Description: e.Description,
			RawData:     e.RawData,
		})
	}

	for _, s := range resp.DiagnosisPath {
		d.DiagnosisPath = append(d.DiagnosisPath, models.DiagnosisStep{
			StepNumber:  s.StepNumber,
			Thought:     s.Thought,
			Action:      s.Action,
			Observation: s.Observation,
			LatencyMs:   s.LatencyMs,
		})
	}

	return d
}
