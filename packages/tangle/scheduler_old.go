package tangle

import (
	"sync"

	"github.com/iotaledger/hive.go/datastructure/set"
	"github.com/iotaledger/hive.go/events"
)

const (
	inboxCapacity = 64
)

// region Scheduler ////////////////////////////////////////////////////////////////////////////////////////////////////

// OldScheduler is a Tangle component that takes care of scheduling the messages that shall be booked.
type OldScheduler struct {
	Events *OldSchedulerEvents

	tangle                 *Tangle
	inbox                  chan MessageID
	scheduledMessages      set.Set
	allMessagesScheduledWG sync.WaitGroup
	shutdownSignal         chan struct{}
	shutdown               sync.WaitGroup
	shutdownOnce           sync.Once
}

// NewOldScheduler returns a new scheduler.
func NewOldScheduler(tangle *Tangle) (scheduler *OldScheduler) {
	scheduler = &OldScheduler{
		Events: &OldSchedulerEvents{
			MessageScheduled: events.NewEvent(MessageIDCaller),
		},

		tangle:            tangle,
		inbox:             make(chan MessageID, inboxCapacity),
		shutdownSignal:    make(chan struct{}),
		scheduledMessages: set.New(true),
	}
	scheduler.run()

	return
}

// Setup sets up the behavior of the component by making it attach to the relevant events of the other components.
func (s *OldScheduler) Setup() {
	s.tangle.Solidifier.Events.MessageSolid.Attach(events.NewClosure(s.Schedule))

	s.tangle.ConsensusManager.Events.MessageOpinionFormed.Attach(events.NewClosure(func(messageID MessageID) {
		if s.scheduledMessages.Delete(messageID) {
			s.allMessagesScheduledWG.Done()
		}
	}))

	s.tangle.Events.MessageInvalid.Attach(events.NewClosure(func(messageID MessageID) {
		if s.scheduledMessages.Delete(messageID) {
			s.allMessagesScheduledWG.Done()
		}
	}))
}

// Schedule schedules the given messageID.
func (s *OldScheduler) Schedule(messageID MessageID) {
	s.inbox <- messageID
}

// Shutdown shuts down the Scheduler and persists its state.
func (s *OldScheduler) Shutdown() {
	s.shutdownOnce.Do(func() {
		close(s.shutdownSignal)
	})

	s.shutdown.Wait()
	s.allMessagesScheduledWG.Wait()
}

func (s *OldScheduler) run() {
	s.shutdown.Add(1)
	go func() {
		defer s.shutdown.Done()

		for {
			select {
			case messageID := <-s.inbox:
				s.scheduleMessage(messageID)
			case <-s.shutdownSignal:
				if len(s.inbox) == 0 {
					return
				}
			}
		}
	}()
}

func (s *OldScheduler) scheduleMessage(messageID MessageID) {
	if !s.parentsBooked(messageID) {
		return
	}

	s.tangle.Storage.MessageMetadata(messageID).Consume(func(messageMetadata *MessageMetadata) {
		if messageMetadata.SetScheduled(true) {
			if s.scheduledMessages.Add(messageID) {
				s.allMessagesScheduledWG.Add(1)
			}
			s.Events.MessageScheduled.Trigger(messageID)
		}
	})
}

func (s *OldScheduler) parentsBooked(messageID MessageID) (parentsBooked bool) {
	s.tangle.Storage.Message(messageID).Consume(func(message *Message) {
		parentsBooked = true
		message.ForEachParent(func(parent Parent) {
			if !parentsBooked || parent.ID == EmptyMessageID {
				return
			}

			if !s.tangle.Storage.MessageMetadata(parent.ID).Consume(func(messageMetadata *MessageMetadata) {
				parentsBooked = messageMetadata.IsBooked()
			}) {
				parentsBooked = false
			}
		})
	})

	return
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region SchedulerEvents /////////////////////////////////////////////////////////////////////////////////////////////

// OldSchedulerEvents represents events happening in the Scheduler.
type OldSchedulerEvents struct {
	// MessageScheduled is triggered when a message is ready to be scheduled.
	MessageScheduled *events.Event
	MessageDiscarded *events.Event
	NodeBlacklisted  *events.Event
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////