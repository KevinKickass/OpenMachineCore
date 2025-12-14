package streaming

import (
    "github.com/google/uuid"
    pb "github.com/KevinKickass/OpenMachineCore/api/proto"
)

type WorkflowService struct {
    pb.UnimplementedWorkflowServiceServer
    streamer *EventStreamer
}

func NewWorkflowService(streamer *EventStreamer) *WorkflowService {
    return &WorkflowService{streamer: streamer}
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
