package streaming

import (
	"context"
	"encoding/json"

	pb "github.com/KevinKickass/OpenMachineCore/api/proto"
	"github.com/KevinKickass/OpenMachineCore/internal/storage"
	"github.com/KevinKickass/OpenMachineCore/internal/workflow/definition"
	"github.com/google/uuid"
)

type WorkflowService struct {
	pb.UnimplementedWorkflowServiceServer
	streamer *EventStreamer
	storage  *storage.PostgresClient
}

func NewWorkflowService(streamer *EventStreamer, storage *storage.PostgresClient) *WorkflowService {
	return &WorkflowService{
		streamer: streamer,
		storage:  storage,
	}
}

func (s *WorkflowService) StreamExecutionStatus(req *pb.ExecutionStreamRequest, stream pb.WorkflowService_StreamExecutionStatusServer) error {
	executionID, err := uuid.Parse(req.ExecutionId)
	if err != nil {
		return err
	}

	eventCh := s.streamer.Subscribe(executionID)
	defer s.streamer.Unsubscribe(executionID, eventCh)

	for {
		select {
		case event, ok := <-eventCh:
			if !ok {
				return nil
			}

			status := &pb.ExecutionStatus{
				ExecutionId: event.ExecutionID.String(),
				EventType:   event.EventType,
				Payload:     string(event.Payload),
				Timestamp:   event.Timestamp.Unix(),
			}

			if err := stream.Send(status); err != nil {
				return err
			}

		case <-stream.Context().Done():
			return stream.Context().Err()
		}
	}
}

func (s *WorkflowService) GetExecutionStatus(ctx context.Context, req *pb.ExecutionStatusRequest) (*pb.ExecutionStatusResponse, error) {
	executionID, err := uuid.Parse(req.ExecutionId)
	if err != nil {
		return nil, err
	}

	// Retrieve execution from storage
	exec, err := s.storage.GetExecution(ctx, executionID)
	if err != nil {
		return nil, err
	}

	// Retrieve execution steps
	steps, err := s.storage.GetExecutionSteps(ctx, executionID)
	if err != nil {
		return nil, err
	}

	// Build response with hierarchical step information
	resp := &pb.ExecutionStatusResponse{
		ExecutionId:   exec.ID.String(),
		WorkflowId:    exec.WorkflowID.String(),
		Status:        string(exec.Status),
		CurrentStep:   int32(exec.CurrentStep),
		CurrentStepId: exec.CurrentStepID,
		Error:         exec.Error,
		StartedAt:     exec.StartedAt.Unix(),
	}

	if exec.CompletedAt != nil {
		resp.CompletedAt = exec.CompletedAt.Unix()
	}

	// Deserialize call stack if available
	if exec.CallStack != nil {
		var callStack []definition.CallFrame
		if err := json.Unmarshal(exec.CallStack, &callStack); err == nil {
			for _, frame := range callStack {
				resp.CallStack = append(resp.CallStack, &pb.CallFrame{
					WorkflowId:  frame.WorkflowID,
					ProgramName: frame.ProgramName,
					StepNumber:  frame.StepNumber,
				})
			}
		}
	}

	// Convert steps to protobuf format with hierarchical information
	for _, step := range steps {
		stepStatus := &pb.StepStatus{
			StepIndex:          int32(step.StepIndex),
			StepName:           step.StepName,
			Status:             string(step.Status),
			Output:             string(step.Output),
			Error:              step.Error,
			HierarchicalStepId: step.HierarchicalStepID,
			Depth:              int32(step.Depth),
		}
		resp.Steps = append(resp.Steps, stepStatus)
	}

	return resp, nil
}
