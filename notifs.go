package main

import (
	"database/sql"
	"fmt"
)

type User struct {
	id         int32
	discord_id string
}

const MASTER_SETTING_KEY = "notifs_enabled"
const PERSIST_TEMP_RECOLLECTION_TIME = 600 // 600 seconds = 10 minutes.

func createTempTable(db *sql.DB) error {
	tempTable := `
        CREATE TEMPORARY TABLE IF NOT EXISTS notifs_sent (
            other_id INTEGER NOT NULL,
            event_type TEXT NOT NULL,
            player_id INTEGER NOT NULL,
            created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
            PRIMARY KEY (other_id, event_type, player_id)
        );`

	_, err := db.Exec(tempTable)
	if err != nil {
		println("Error creating temporary table 'notifs_sent':", err.Error())
	}
	return err
}

func markNotifSent(db *sql.DB, otherID int64, eventType string, playerID int32) error {
	query := `
		INSERT INTO notifs_sent (other_id, event_type, player_id)
		VALUES (?, ?, ?)
		ON CONFLICT(other_id, event_type, player_id) DO NOTHING;`

	_, err := db.Exec(query, otherID, eventType, playerID)
	if err != nil {
		println("Error marking notification as sent for otherID", otherID, "eventType", eventType, "playerID", playerID, ":", err.Error())
	}
	return err
}

func hasNotifBeenSent(db *sql.DB, otherID int64, eventType string, playerID int32) (bool, error) {
	query := `
		SELECT EXISTS(SELECT 1 FROM notifs_sent WHERE other_id = ? AND event_type = ? AND player_id = ?)`

	var exists bool
	err := db.QueryRow(query, otherID, eventType, playerID).Scan(&exists)

	if err != nil {
		println("Error checking if notification has been sent for otherID", otherID, "eventType", eventType, "playerID", playerID, ":", err.Error())
		return false, err
	}

	return exists, err
}

func purgeOldNotifs(db *sql.DB) error {
	modifier := fmt.Sprintf("-%d seconds", PERSIST_TEMP_RECOLLECTION_TIME)
	query := `
		DELETE FROM notifs_sent
		WHERE created_at < datetime('now', ?);`
	_, err := db.Exec(query, modifier)
	if err != nil {
		println("Error purging old notifications from 'notifs_sent':", err.Error())
	}
	return err
}

func dumpNotifsSent(db *sql.DB) {
	tableName := "notifs_sent"
	// 1. Execute the query using dynamic table naming
	// (Note: Ensure tableName is sanitized if it comes from user input to prevent SQL injection)
	query := fmt.Sprintf("SELECT * FROM %s", tableName)
	rows, err := db.Query(query)
	if err != nil {
		println("Error querying table", tableName, ":", err.Error())
		return
	}
	defer rows.Close()

	// 2. Get column names dynamically
	columns, err := rows.Columns()
	if err != nil {
		println("Error getting column names for table", tableName, ":", err.Error())
		return
	}

	// Print column headers
	fmt.Printf("--- DUMP OF TABLE: %s ---\n", tableName)
	fmt.Println(columns)

	// 3. Prepare slices to hold row values dynamically
	values := make([]interface{}, len(columns))
	valuePtrs := make([]interface{}, len(columns))
	for i := range columns {
		valuePtrs[i] = &values[i]
	}

	// 4. Iterate through the rows
	rowCount := 0
	for rows.Next() {
		rowCount++
		err := rows.Scan(valuePtrs...)
		if err != nil {
			println("Error scanning row", rowCount, "for table", tableName, ":", err.Error())
			return
		}

		// 5. Process and print each column value
		fmt.Printf("Row %d: ", rowCount)
		for i, val := range values {
			if val == nil {
				fmt.Printf("[%s: NULL] ", columns[i])
			} else {
				switch b := val.(type) {
				case []byte:
					fmt.Printf("[%s: %s] ", columns[i], string(b))
				default:
					fmt.Printf("[%s: %v] ", columns[i], b)
				}
			}
		}
		fmt.Println()
	}

	if err = rows.Err(); err != nil {
		println("Error iterating through rows for table", tableName, ":", err.Error())
		return
	}

	fmt.Printf("--- END OF DUMP (%d rows) ---\n", rowCount)
}

func getUsersToNotify(db *sql.DB, eventType string) ([]User, error) {
	query := `
		SELECT id, discord_id FROM players
		JOIN settings ON players.id = settings.player_id
		WHERE settings.setting = ? AND settings.value = 'true' AND players.id IN (SELECT player_id FROM settings WHERE setting = ? AND value = 'true');`

	rows, err := db.Query(query, eventType, MASTER_SETTING_KEY)
	if err != nil {
		println("Error querying for users to notify for event type", eventType, ":", err.Error())
		return nil, err
	}
	defer rows.Close()

	users := []User{}
	for rows.Next() {
		var userID int32
		var discordID string
		err := rows.Scan(&userID, &discordID)
		if err != nil {
			println("Error scanning user row for event type", eventType, ":", err.Error())
			return nil, err
		}
		users = append(users, User{id: userID, discord_id: discordID})
	}
	return users, nil
}
