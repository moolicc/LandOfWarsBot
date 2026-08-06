package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

type EventType interface {
	// Returns the name of the type of event.
	Name() string

	// Return the key of the event's enable/disable setting.
	SettingKey() string

	// Checks if the event has occurred for the given user and returns a message embed for sending later if it has.
	CheckEvent(db *sql.DB, forUser int32, cursor time.Time) (bool, discordgo.MessageEmbed)
}

type BuildingCompletedEvent struct {
}

type AttackStartedEvent struct {
}

const FOOTER_TEXT = "Note: You can disable this notification in the game"

// ======================================================================================
// Building complete
// ======================================================================================

func (e BuildingCompletedEvent) Name() string {
	return "Building Completed Event"
}

func (e BuildingCompletedEvent) SettingKey() string {
	return "notif_building_completed_event_enabled"
}

func (e BuildingCompletedEvent) CheckEvent(db *sql.DB, forUser int32, cursor time.Time) (bool, discordgo.MessageEmbed) {

	actions, err := getActionsForUser(db, forUser, cursor.Unix())
	if err != nil {
		println("Error getting actions for user", forUser, ":", err.Error())
		return false, discordgo.MessageEmbed{}
	}

	if len(actions) == 0 {
		return false, discordgo.MessageEmbed{}
	}

	buildingsInTowns := []string{}
	for _, action := range actions {
		// Make sure a notification hasn't already been sent for this event.
		wasNotifSent, err := hasNotifBeenSent(db, action.id, e.Name(), forUser)

		if wasNotifSent || err != nil {
			println("...Notification already sent for action (otherid:", action.id, ", name:", e.Name(), ", user:", forUser, ")")
			if err != nil {
				println("Error checking if notification was sent for action", action.id, ":", err.Error())
			}
			continue
		}

		// Parse the result json to get the building type and level.
		var resultData map[string]interface{}
		err = json.Unmarshal([]byte(action.result.String), &resultData)
		if err != nil {
			println("Error parsing result JSON for action", action.id, ":", err.Error())
			continue
		}

		// Find the type of building and its level from the result data.
		buildingType, ok := resultData["building_type"].(string)
		if !ok {
			continue
		}

		level, ok := resultData["level"].(float64)
		if !ok {
			continue
		}

		// Find the town the building was constructed in.
		town, err := getTown(db, int32(action.town_id.Int64))
		if err != nil {
			println("Error getting town for action", action.id, ":", err.Error())
			buildingsInTowns = append(buildingsInTowns, fmt.Sprintf(" * **%s** (Level %d)", buildingType, int(level)))
			continue
		}

		buildingsInTowns = append(buildingsInTowns, fmt.Sprintf(" * **%s** (Level %d) in *%s*", buildingType, int(level), town.name))

		// Ensure if the total length is small enough for a message field.
		totalLength := 0

		// We breakout if the total length exceeds the maximum message length for a field.
		// This way we don't mark the notification as sent so that we can resend it later..
		breakOut := false

		for _, b := range buildingsInTowns {
			curLen := len(b) + 1 // +1 for newline

			if totalLength+curLen > MAX_MESSAGE_LENGTH {
				buildingsInTowns = buildingsInTowns[:len(buildingsInTowns)-1]
				breakOut = true
				break
			}
			totalLength += curLen //+ 1 for newline
		}

		// ... the part where we break out to avoid marking it as sent in the event the message is already too long.
		if breakOut {
			break
		}

		// Mark the notification as sent.
		println("...Marking notification as sent for action (otherid:", action.id, ", name:", e.Name(), ", user:", forUser, ")")
		markNotifSent(db, action.id, e.Name(), forUser)
	}

	if len(buildingsInTowns) == 0 {
		return false, discordgo.MessageEmbed{}
	}

	// Create a message embed to send to the user.
	embed := discordgo.MessageEmbed{
		Title: "Building(s) Completed",
		//Description: "The following buildings have been completed",
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "",
				Value:  strings.Join(buildingsInTowns, "\n"),
				Inline: true,
			},
		},
		Author: &discordgo.MessageEmbedAuthor{
			Name: "Lands of War",
		},
		Timestamp: time.Now().Format(time.RFC3339),
		Footer: &discordgo.MessageEmbedFooter{
			Text: FOOTER_TEXT,
		},
	}

	return true, embed
}

// ======================================================================================
// Attack started
// ======================================================================================

// {"from_town_id":3,"to_town_id":2,"units":{"archer":99,"catapult":61,"horseman":888,"man_at_arms":73,"swordsman":197},"attack_type":"attack"}

func (e AttackStartedEvent) Name() string {
	return "Atack Started Event"
}

func (e AttackStartedEvent) SettingKey() string {
	return "notif_attack_started_event_enabled"
}

func (e AttackStartedEvent) CheckEvent(db *sql.DB, forUser int32, cursor time.Time) (bool, discordgo.MessageEmbed) {

	// Send one message with all incoming attacks:
	//   Title: Under attack!
	//   Desc: You are under attack!
	//   Fields:
	//     From [attacking player name] of the [realm name] realm.
	//     [# units] arriving <t:1785289955:R>

	// Find the user's towns.
	towns, err := getTowns(db, forUser)
	if err != nil {
		println("Error getting towns for user for attack_march event", forUser, ":", err.Error())
		return false, discordgo.MessageEmbed{}
	}

	townIds := []int32{}
	for _, t := range towns {
		townIds = append(townIds, t.id)
	}

	if len(townIds) == 0 {
		println("No towns found for user", forUser, "for attack_march event")
		return false, discordgo.MessageEmbed{}
	}

	// We want attacks on our towns that have started after the cursor, but also finish after the current time (for incoming attacks).
	// The goal is to send notifications for new attacks.

	townIdsFormatted := "(" + strings.Trim(strings.Join(strings.Fields(fmt.Sprint(townIds)), ","), "[]") + ")"
	query := fmt.Sprintf(`SELECT * FROM actions WHERE started_at >= ? and completes_at >= ? and action_type = 'attack_march' and json_extract(data, '$.to_town_id') in %s ORDER BY completed_at DESC;`, townIdsFormatted)
	println("Querying for attack_march actions for user", forUser, "between times", cursor.Unix(), "and", time.Now().UTC().Unix(), "with query:", query)
	rows, err := db.Query(query, cursor.Unix(), time.Now().UTC().Unix(), townIdsFormatted)

	if err != nil {
		println("Error querying for attack_march actions for user", forUser, ":", err.Error())
		return false, discordgo.MessageEmbed{}
	}

	actions, err := createActions(rows)
	if err != nil {
		println("Error creating actions for attack_march actions for user", forUser, ":", err.Error())
		return false, discordgo.MessageEmbed{}
	}
	if len(actions) == 0 {
		return false, discordgo.MessageEmbed{}
	}

	// We need to find all attacks incoming to the user's towns.
	// We then need to build a message list for all the incoming attacks (that we haven't yet sent a message about).

	messageTitles := []string{}
	messageLines := []string{}

	for _, action := range actions {
		// Make sure a notification hasn't already been sent for this event.
		wasNotifSent, err := hasNotifBeenSent(db, action.id, e.Name(), forUser)

		if wasNotifSent || err != nil {
			println("...Notification already sent for action (otherid:", action.id, ", name:", e.Name(), ", user:", forUser, ")")
			if err != nil {
				println("Error checking if notification was sent for action", action.id, ":", err.Error())
			}
			continue
		}

		// Parse the result json to get the attack information
		var resultData map[string]interface{}
		err = json.Unmarshal([]byte(action.data.String), &resultData)
		if err != nil {
			println("Error parsing result JSON for action", action.id, ":", err.Error())
			continue
		}

		// Find the realm and attacking player names.
		playerAndRealm, err := getPlayerAndRealm(db, action.player_id)
		if err != nil {
			println("Error getting player and realm for action", action.id, " and for player", action.player_id, ":", err.Error())
			continue
		}

		// Find the total number of attacking units.
		totalUnitCount := 0
		if units, ok := resultData["units"].(map[string]interface{}); ok {
			for _, val := range units {
				if count, ok := val.(float64); ok {
					totalUnitCount += int(count)
				}
			}
		} else {
			println("Error: units data is not in expected format for action", action.id)
			continue
		}

		toTownId := resultData["to_town_id"].(float64)
		townName := ""
		for _, t := range towns {
			if t.id == int32(toTownId) {
				townName = t.name
				break
			}
		}

		messageTitles = append(messageTitles, fmt.Sprintf("From %s of the %s realm", playerAndRealm.player_name, playerAndRealm.realm_name))
		messageLines = append(messageLines, fmt.Sprintf("%d units attacking %s <t:%d:R>", totalUnitCount, townName, action.completes_at))

		// Mark the notification as sent.
		println("...Marking notification as sent for action (otherid:", action.id, ", name:", e.Name(), ", user:", forUser, ")")
		markNotifSent(db, action.id, e.Name(), forUser)
	}

	fields := []*discordgo.MessageEmbedField{}
	for i := 0; i < len(messageTitles); i++ {
		fields = append(fields, &discordgo.MessageEmbedField{
			Name:   messageTitles[i],
			Value:  messageLines[i],
			Inline: true,
		})
	}

	// Create a message embed to send to the user.
	embed := discordgo.MessageEmbed{
		Title:       "Incoming Attack!",
		Description: "You are under attack!",
		Fields:      fields,
		Author: &discordgo.MessageEmbedAuthor{
			Name: "Lands of War",
		},
		Timestamp: time.Now().Format(time.RFC3339),
		Footer: &discordgo.MessageEmbedFooter{
			Text: FOOTER_TEXT,
		},
	}

	return true, embed
}
