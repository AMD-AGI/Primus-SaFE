package model

const (
	ChannelEmail = "email"
)

const (
	TopicWorkload = "workload"
)

type Message struct {
	Email *EmailMessage
}

func (m Message) GetChannels() []string {
	channels := []string{}
	if m.Email != nil {
		channels = append(channels, ChannelEmail)
	}
	return channels
}

type EmailMessage struct {
	To      []string
	Title   string
	Content string
}
