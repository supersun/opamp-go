package internal

import (
	"context"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"

	"github.com/supersun/opamp-go/client/types"
	"github.com/supersun/opamp-go/internal"
	"github.com/supersun/opamp-go/protobufs"
)

const (
	defaultSendCloseMessageTimeout = 5 * time.Second
)

// WSSender implements the WebSocket client's sending portion of OpAMP protocol.
type WSSender struct {
	SenderCommon
	conn   *websocket.Conn
	logger types.Logger

	// Indicates that the sender has fully stopped.
	stopped chan struct{}
	err     error
}

// NewSender creates a new Sender that uses WebSocket to send
// messages to the server.
func NewSender(logger types.Logger) *WSSender {
	return &WSSender{
		logger:       logger,
		SenderCommon: NewSenderCommon(),
	}
}

// Start the sender and send the first message that was set via NextMessage().Update()
// earlier. To stop the WSSender cancel the ctx.
func (s *WSSender) Start(ctx context.Context, conn *websocket.Conn) error {
	s.conn = conn
	err := s.sendNextMessage(ctx)

	// Run the sender in the background.
	s.stopped = make(chan struct{})
	s.err = nil
	go s.run(ctx)

	return err
}

// IsStopped returns a channel that's closed when the sender is stopped.
func (s *WSSender) IsStopped() <-chan struct{} {
	return s.stopped
}

// StoppingErr returns an error if there was a problem with stopping the sender.
// If stopping was successful will return nil.
// StoppingErr() can be called only after IsStopped() is signalled.
func (s *WSSender) StoppingErr() error {
	return s.err
}

func (s *WSSender) run(ctx context.Context) {
out:
	for {
		select {
		case <-s.hasPendingMessage:
			s.sendNextMessage(ctx)

		case <-ctx.Done():
			if err := s.sendCloseMessage(); err != nil && err != websocket.ErrCloseSent {
				s.err = err
			}
			break out
		}
	}

	close(s.stopped)
}

func (s *WSSender) sendCloseMessage() error {
	return s.conn.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, "Normal closure"),
		time.Now().Add(defaultSendCloseMessageTimeout),
	)
}

func (s *WSSender) sendNextMessage(ctx context.Context) error {
	msgToSend := s.nextMessage.PopPending()
	if msgToSend != nil && !proto.Equal(msgToSend, &protobufs.AgentToServer{}) {
		// There is a pending message and the message has some fields populated.
		return s.sendMessage(ctx, msgToSend)
	}
	return nil
}

func (s *WSSender) sendMessage(ctx context.Context, msg *protobufs.AgentToServer) error {
	if err := internal.WriteWSMessage(s.conn, msg); err != nil {
		s.logger.Errorf(ctx, "Cannot write WS message: %v", err)
		// TODO: check if it is a connection error then propagate error back to Client and reconnect.
		return err
	}
	return nil
}
