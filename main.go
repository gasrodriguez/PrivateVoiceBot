package main

import (
	"fmt"
	"github.com/bwmarrin/discordgo"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"
	"time"
)

// Type for voice channels in guilds made with this bot
type voiceChannel struct {
	GuildID         string
	ChannelID       string
	ParentChannelID string
	OwnerID         string
	Name            string
	OPs             []string
	CreatedAt       time.Time
}

func (o *voiceChannel) IsExpired() bool {
	return o.CreatedAt.Add(channelDeleteDelay).Before(time.Now())
}

func (o *voiceChannel) Delete() {
	_, err := dg.ChannelDelete(o.ChannelID)
	checkError(err)
}

func (o *voiceChannel) ConfirmDelete() {
	delete(channels, o.ChannelID)
	_, err := dg.ChannelMessageSend(o.ParentChannelID, "Deleted voice channel `"+o.Name+"`")
	checkError(err)
}

var (
	// Session for discordgo
	dg *discordgo.Session

	// err so it can be referenced outside of main()
	// Stands for Global err
	gerr error

	// Bot user after session is created.
	u *discordgo.User

	// Channels made with the bot
	channels map[string]*voiceChannel

	// Commands users can run
	commands = [8]string{"meet"}

	// Items read from settings.ini
	// Token for the discordgo Session.
	token string
	port  string

	// Prefix the bot looks for in a message
	prefix string

	// Prefix for voice channels made by the bot
	voicePrefix string

	// Delay (in seconds) for the ticker to wait for.
	tickerDelay time.Duration

	// How long a channel should be allowed to stay unjoined until it is deleted
	channelDeleteDelay time.Duration
)

func checkError(err error) {
	if err != nil {
		panic(err)
	}
}

func catchPanic() {
	if r := recover(); r != nil {
		fmt.Printf("%s", r)
		debug.PrintStack()
	}
}

func main() {
	var err error
	var ok bool

	token, ok = os.LookupEnv("TOKEN")
	if !ok {
		panic("Environment var 'TOKEN' not set")
	}

	port, ok = os.LookupEnv("PORT")
	if !ok {
		panic("Environment var 'PORT' not set")
	}

	prefix = "!"
	voicePrefix = "PV: "
	tickerDelay = 10 * time.Second
	channelDeleteDelay = 30 * time.Second

	// Create the discordgo Session.
	dg, err = discordgo.New("Bot " + token)
	checkError(err)

	// Get the account information.
	u, gerr = dg.User("@me")
	checkError(gerr)

	// Make the bot call messageCreate() whenever the event is ran in Discord
	dg.AddHandler(messageCreate)
	dg.AddHandler(channelDelete)

	// Initialize the map for the voice channels made by the bot
	channels = make(map[string]*voiceChannel)

	dg.Identify.Intents = discordgo.MakeIntent(discordgo.IntentsAll)

	// Open up the discordgo Session websocket
	err = dg.Open()
	checkError(err)

	// Get a ticker to check every X (default: 30) seconds if a non-joined voice-channel should expire.
	ticker := time.NewTicker(tickerDelay)
	quit := make(chan struct{})
	go func() {
		defer catchPanic()
		for {
			select {
			case <-ticker.C:
				// Checking all the channels for ones who's overstayed their welcome
				// By that, channels who's timestamps are "expired."
				for id, channel := range channels {
					guild, err := dg.State.Guild(channel.GuildID)
					checkError(err)
					connected := 0
					for _, voiceState := range guild.VoiceStates {
						if voiceState.ChannelID == id {
							connected++
						}
					}
					if channel.IsExpired() && connected == 0 {
						channel.Delete()
					}
				}
			case <-quit:
				ticker.Stop()
				return
			}
		}
	}()

	go func() {
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
		fmt.Println("Bot is now running.  Press CTRL-C to exit.\n---")
		<-signals
		fmt.Println("Shutting down bot.")
		os.Exit(0)
	}()

	http.HandleFunc("/", ping)
	err = http.ListenAndServe(":"+port, nil)
	checkError(err)

}

func ping(w http.ResponseWriter, _ *http.Request) {
	_, err := w.Write([]byte("pong"))
	checkError(err)
}

func stringInSlice(a string, slice []string) bool {
	for _, b := range slice {
		if b == a {
			return true
		}
	}
	return false
}

// Ran whenever a message is sent in a text channel the bot has access to.
func messageCreate(s *discordgo.Session, message *discordgo.MessageCreate) {
	defer catchPanic()

	// If the message author is the bot, ignore it.
	if message.Author.ID == u.ID {
		return
	}

	// Ignore if the message is empty (in the case of images being sent.)
	if len(message.Content) == 0 {
		return
	}

	// If the message does not begin with the prefix the user set, ignore it.]
	if string(message.Content[0]) != prefix {
		return
	}

	// Explode the command so we can look at some of the stuff a bit easier
	// TODO: Possibly make this a bit more streamlined since I don't use a fair portion of this.
	explodedCommand := strings.Split(message.Content[1:], " ")
	baseCommand := strings.ToLower(explodedCommand[0])

	// Not for us
	if !stringInSlice(baseCommand, commands[:]) {
		return
	}

	switch baseCommand {
	case "meet":
		makeNewPrivateVoice(s, strings.Join(explodedCommand[1:], " "), message)
		break
	}
}

func makeNewPrivateVoice(s *discordgo.Session, title string, message *discordgo.MessageCreate) {
	// Check to see if the title can fit in Discord's bounds.
	if len(voicePrefix)+len(title) > 100 {
		// If the title can't fit, bitch at the caster.
		_, err := s.ChannelMessageSend(message.ChannelID, "<@"+message.Author.ID+">, that does not fit!")
		checkError(err)
	}

	parentChannel, err := s.Channel(message.ChannelID)
	checkError(err)

	if parentChannel == nil {
		_, err := s.ChannelMessageSend(message.ChannelID, "Cannot create channel here.")
		checkError(err)
		return
	}

	// Check if this parent channel has already a child voice channel
	for _, v := range channels {
		if v.ParentChannelID == message.ChannelID {
			_, err := s.ChannelMessageSend(message.ChannelID, "There is already a voice channel `"+v.Name+"`")
			checkError(err)
			return
		}
	}

	if len(title) == 0 {
		title = parentChannel.Name
	}

	newChannel, err := s.GuildChannelCreate(message.GuildID, voicePrefix+title, discordgo.ChannelTypeGuildVoice)
	checkError(err)

	channels[newChannel.ID] = &voiceChannel{
		GuildID:         message.GuildID,
		ChannelID:       newChannel.ID,
		ParentChannelID: message.ChannelID,
		OwnerID:         message.Author.ID,
		Name:            voicePrefix + title,
		OPs:             []string{},
		CreatedAt:       time.Now(),
	}
	_, err = s.ChannelMessageSend(message.ChannelID, "Created voice channel `"+channels[newChannel.ID].Name+"`")
	checkError(err)

	// Set @everyone to not being able to connect or do anything with that voice channel
	err = s.ChannelPermissionSet(newChannel.ID, message.GuildID, discordgo.PermissionOverwriteTypeRole, 0, 66060288)
	checkError(err)

	guild, err := s.State.Guild(message.GuildID)
	checkError(err)

	for _, member := range guild.Members {
		perm, err := s.State.UserChannelPermissions(member.User.ID, parentChannel.ID)
		checkError(err)
		if perm&discordgo.PermissionViewChannel == 0 {
			continue
		}
		// Set the owner to have some basic administration rights and to be able to connect
		err = s.ChannelPermissionSet(newChannel.ID, member.User.ID, discordgo.PermissionOverwriteTypeMember, discordgo.PermissionAllVoice, 0)
		checkError(err)
	}
}

func channelDelete(_ *discordgo.Session, channelDelete *discordgo.ChannelDelete) {
	defer catchPanic()
	channel, ok := channels[channelDelete.Channel.ID]
	if ok {
		channel.ConfirmDelete()
	}
}
