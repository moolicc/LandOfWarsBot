package main

import (
	"database/sql"
)

type action struct {
	id           int64
	player_id    int32
	town_id      sql.NullInt64
	action_type  string
	data         sql.NullString
	status       sql.NullString
	started_at   int64
	completes_at int64
	completed_at sql.NullInt64
	result       sql.NullString
}

type town struct {
	id                 int32
	player_id          int32
	name               string
	x                  int32
	y                  int32
	population         int32
	gold               int32
	food               int32
	wood               int32
	stone              int32
	iron               int32
	last_resource_tick int64
	created_at         int64
	morale             int64
	town_type          string
	country_id         string
}

type player struct {
	id         int32
	discord_id string
	username   string
	avatar_url string
	country_id string
	realm_id   int32
}

type realm struct {
	id   int32
	name string
}

type playerWithRealm struct {
	player_id   int32
	realm_id    int32
	player_name string
	realm_name  string
}

func getActionsForUser(db *sql.DB, userID int32, cursor int64) ([]action, error) {
	query := `
	select * from actions
	where (action_type = 'building_construct' or action_type = 'building_upgrade') and player_id = ? and completed_at >= ?`

	println("Querying for actions for user", userID, "with cursor", cursor)

	rows, err := db.Query(query, userID, cursor)
	if err != nil {
		println("Error querying for actions for user", userID, ":", err.Error())
		return nil, err
	}
	defer rows.Close()

	actions := []action{}
	for rows.Next() {
		var a action
		err := rows.Scan(&a.id, &a.player_id, &a.town_id, &a.action_type, &a.data, &a.status, &a.started_at, &a.completes_at, &a.completed_at, &a.result)
		if err != nil {
			println("Error scanning action row for user", userID, ":", err.Error())
			return nil, err
		}
		actions = append(actions, a)
	}

	return actions, nil
}

func createActions(rows *sql.Rows) ([]action, error) {
	actions := []action{}

	var err error
	for rows.Next() {
		var a action
		err = rows.Scan(&a.id, &a.player_id, &a.town_id, &a.action_type, &a.data, &a.status, &a.started_at, &a.completes_at, &a.completed_at, &a.result)
		if err != nil {
			continue
		}
		actions = append(actions, a)
	}
	return actions, err
}

func getTown(db *sql.DB, townID int32) (town, error) {
	query := `
	select * from towns where id = ?;`
	rows, err := db.Query(query, townID)

	if err != nil {
		println("Error querying for town with ID", townID, ":", err.Error())
		return town{}, err
	}
	defer rows.Close()

	towns := []town{}
	for rows.Next() {
		var t town
		err := rows.Scan(&t.id, &t.player_id, &t.name, &t.x, &t.y, &t.population, &t.gold, &t.food, &t.wood, &t.stone, &t.iron, &t.last_resource_tick, &t.created_at, &t.morale, &t.town_type, &t.country_id)
		if err != nil {
			println("Error scanning town row for town with ID", townID, ":", err.Error())
			return town{}, err
		}

		towns = append(towns, t)
	}
	if len(towns) == 0 {
		println("No town found with ID", townID)
		return town{}, nil
	}

	return towns[0], nil
}

func getTowns(db *sql.DB, userID int32) ([]town, error) {
	query := `
	select * from towns where player_id = ?;`
	rows, err := db.Query(query, userID)
	if err != nil {
		println("Error querying for towns for user with ID", userID, ":", err.Error())
		return nil, err
	}
	defer rows.Close()

	towns := []town{}
	for rows.Next() {
		var t town
		err := rows.Scan(&t.id, &t.player_id, &t.name, &t.x, &t.y, &t.population, &t.gold, &t.food, &t.wood, &t.stone, &t.iron, &t.last_resource_tick, &t.created_at, &t.morale, &t.town_type, &t.country_id)
		if err != nil {
			return nil, err
		}
		towns = append(towns, t)
	}

	return towns, nil
}

func getPlayer(db *sql.DB, playerID int32) (player, error) {
	query := `
	select * from players where id = ?;`
	rows, err := db.Query(query, playerID)

	if err != nil {
		println("Error querying for player with ID", playerID, ":", err.Error())
		return player{}, err
	}
	defer rows.Close()

	var p player
	err = rows.Scan(&p.id, &p.discord_id, &p.username, &p.avatar_url, &p.country_id, &p.realm_id)
	if err != nil {
		println("Error scanning player row for player with ID", playerID, ":", err.Error())
		return player{}, err
	}
	return p, nil

}

func getPlayerAndRealm(db *sql.DB, playerID int32) (playerWithRealm, error) {
	query := `
	select p.id as player_id, p.username as player_name, r.id as realm_id, r.name as realm_name from players p JOIN realms r ON p.realm_id = r.id WHERE p.id = ?;`

	rows, err := db.Query(query, playerID)
	if err != nil {
		println("Error querying for player and town with ID", playerID, ":", err.Error())
		return playerWithRealm{}, err
	}
	defer rows.Close()

	result := playerWithRealm{}
	rows.Next()
	err = rows.Scan(&result.player_id, &result.player_name, &result.realm_id, &result.realm_name)
	if err != nil {
		println("Error scanning player and town row for player with ID", playerID, ":", err.Error())
		return playerWithRealm{}, err
	}

	return result, nil
}
