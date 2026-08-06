package main

// Invite: https://discord.com/oauth2/authorize?client_id=1482827946107605143&scope=bot&permissions=8515702525262912
// Buildings "result" json needs to have the correct level for the completed building

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	_ "modernc.org/sqlite"

	"github.com/bwmarrin/discordgo"
	//_ "github.com/mattn/go-sqlite3"
)

type UserNotification struct {
	user    User
	message discordgo.MessageEmbed
}

const TICK_INTERVAL = 90 // seconds
const MAX_MESSAGE_LENGTH = 1024

func main() {

	println("Starting bot...")
	// Get environment variables
	Token := os.Getenv("DISCORD_BOT_TOKEN")
	if Token == "" {
		log.Fatal("DISCORD_BOT_TOKEN environment variable not set")
		return
	}

	DbPath := os.Getenv("DB_PATH")
	if DbPath == "" {
		log.Fatal("DB_PATH environment variable not set")
		return
	}

	// Open the database connection
	Db, err := sql.Open("sqlite", DbPath)
	if err != nil {
		log.Fatal("Error opening database: ", err)
		return
	}
	defer Db.Close()

	err = Db.Ping()
	if err != nil {
		log.Fatal("Error pinging database: ", err)
		return
	}

	// Create the temp db table to store sent notifications.
	err = createTempTable(Db)
	if err != nil {
		log.Fatal("Error creating temp table: ", err)
		return
	}

	println("Connected to db!")

	// Open the Discord session
	dg, err := discordgo.New("Bot " + Token)
	if err != nil {
		log.Fatal("Error creating Discord session: ", err)
		return
	}

	err = dg.Open()
	if err != nil {
		log.Fatal("Error opening discord connection: ", err)
		return
	}
	defer dg.Close()
	println("Connected to discord!")

	fmt.Println("Bot is now running. Press CTRL-C to exit.")
	run(Db, dg)
}

var lastTick time.Time

func run(db *sql.DB, dg *discordgo.Session) {
	lastTick = time.Now().UTC().Add(-TICK_INTERVAL * time.Second)
	for {
		time.Sleep(TICK_INTERVAL * time.Second)
		messages := checkEvents(db)
		lastTick = time.Now().UTC().Add(-TICK_INTERVAL * time.Second)

		sendMessages(dg, messages)

		dumpNotifsSent(db)
		purgeOldNotifs(db)
		dumpNotifsSent(db)
	}
}

var eventTypes = []EventType{
	BuildingCompletedEvent{},
	AttackStartedEvent{},
}

func checkEvents(db *sql.DB) []UserNotification {
	results := []UserNotification{}

	for _, eventType := range eventTypes {
		usersToNotify, err := getUsersToNotify(db, eventType.SettingKey())
		if err != nil {
			log.Println("Error getting users to notify for event type", eventType.Name(), ":", err.Error())
			continue
		}

		println(len(usersToNotify), "users to check for [", eventType.Name(), "] since", lastTick.Format(time.RFC3339))
		if len(usersToNotify) > 0 {
			println(usersToNotify[0].id, "discord_id:", usersToNotify[0].discord_id)
		}

		for _, user := range usersToNotify {
			println("...User:", user.id, "discord_id:", user.discord_id)

			success, result := eventType.CheckEvent(db, user.id, lastTick)

			if success {
				cur := len(results)
				results = append(results, UserNotification{user: user, message: result})
				println("...User has", len(results)-cur, "notifications")
			} else {
				println("...User has no new notifications")
			}
		}
	}

	return results
}

func sendMessages(dg *discordgo.Session, messages []UserNotification) {

	messagesByUser := make(map[string][]*discordgo.MessageEmbed)
	usersByDiscordId := make(map[string]User)
	channelMap := make(map[string]string)

	// Group the messages by user and resolve user channel IDs.
	for _, message := range messages {
		usersByDiscordId[message.user.discord_id] = message.user
		messagesByUser[message.user.discord_id] = append(messagesByUser[message.user.discord_id], &message.message)
		_, ok := channelMap[message.user.discord_id]
		if !ok {
			println("...Creating DM channel for user: ", message.user.id, "discord id: ", message.user.discord_id, "for event notification", message.message.Title)
			channel, err := dg.UserChannelCreate(message.user.discord_id)
			if err != nil {
				log.Println("Error creating DM channel for user", message.user.discord_id, ":", err.Error())
				continue
			}
			channelMap[message.user.discord_id] = channel.ID
		}
	}

	for userID, userMessages := range messagesByUser {
		internalUserId := usersByDiscordId[userID].id
		channelId, ok := channelMap[userID]
		if !ok {
			log.Println("No channel ID found for user", internalUserId)
			continue
		}

		verifyMessageLength(userMessages)
		_, err := dg.ChannelMessageSendEmbeds(channelId, userMessages)
		if err != nil {
			log.Println("Error sending message to user", internalUserId, ":", err.Error())
		}
	}
}

func verifyMessageLength(messages []*discordgo.MessageEmbed) {
	for _, message := range messages {
		for _, field := range message.Fields {
			if len(field.Value) > MAX_MESSAGE_LENGTH {
				field.Value = field.Value[:MAX_MESSAGE_LENGTH-3] + "..."
			}
		}
	}
}
