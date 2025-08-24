package services

// TeamData holds team information including logo URLs
type TeamData struct {
	Name    string
	City    string
	IconURL string
}

// GetTeamData returns team information for all NFL teams
// Using the same Google Static CDN URLs as the legacy application
func GetTeamData() map[string]TeamData {
	return map[string]TeamData{
		// AFC East
		"BUF": {
			Name:    "Bills",
			City:    "Buffalo",
			IconURL: "https://ssl.gstatic.com/onebox/media/sports/logos/_RMCkIDTISqCPcSoEvRDhg_48x48.png",
		},
		"MIA": {
			Name:    "Dolphins",
			City:    "Miami",
			IconURL: "https://ssl.gstatic.com/onebox/media/sports/logos/1ysKnl7VwOQO8g94gbjKdQ_48x48.png",
		},
		"NE": {
			Name:    "Patriots",
			City:    "New England",
			IconURL: "https://ssl.gstatic.com/onebox/media/sports/logos/z89hPEH9DZbpIYmF72gSaw_48x48.png",
		},
		"NYJ": {
			Name:    "Jets",
			City:    "New York",
			IconURL: "https://ssl.gstatic.com/onebox/media/sports/logos/T4TxwDGkrCfTrL6Flg9ktQ_48x48.png",
		},
		
		// AFC North
		"BAL": {
			Name:    "Ravens",
			City:    "Baltimore",
			IconURL: "https://ssl.gstatic.com/onebox/media/sports/logos/1vlEqqoyb9uTqBYiBeNH-w_48x48.png",
		},
		"CIN": {
			Name:    "Bengals",
			City:    "Cincinnati",
			IconURL: "https://ssl.gstatic.com/onebox/media/sports/logos/wDDRqMa40nidAOA5883Vmw_48x48.png",
		},
		"CLE": {
			Name:    "Browns",
			City:    "Cleveland",
			IconURL: "https://ssl.gstatic.com/onebox/media/sports/logos/bTzlW33n9s53DxRzmlZXyg_48x48.png",
		},
		"PIT": {
			Name:    "Steelers",
			City:    "Pittsburgh",
			IconURL: "https://ssl.gstatic.com/onebox/media/sports/logos/mdUFLAswQ4jZ6V7jXqaxig_48x48.png",
		},
		
		// AFC South
		"HOU": {
			Name:    "Texans",
			City:    "Houston",
			IconURL: "https://ssl.gstatic.com/onebox/media/sports/logos/sSUn9HRpYLQtEFF2aG9T8Q_48x48.png",
		},
		"IND": {
			Name:    "Colts",
			City:    "Indianapolis",
			IconURL: "https://ssl.gstatic.com/onebox/media/sports/logos/zOE7BhKadEjaSrrFjcnR4w_48x48.png",
		},
		"JAX": {
			Name:    "Jaguars",
			City:    "Jacksonville",
			IconURL: "https://ssl.gstatic.com/onebox/media/sports/logos/HLfqVCxzVx5CUDQ07GLeWg_48x48.png",
		},
		"TEN": {
			Name:    "Titans",
			City:    "Tennessee",
			IconURL: "https://ssl.gstatic.com/onebox/media/sports/logos/9J9dhhLeSa3syZ1bWXRjaw_48x48.png",
		},
		
		// AFC West
		"DEN": {
			Name:    "Broncos",
			City:    "Denver",
			IconURL: "https://ssl.gstatic.com/onebox/media/sports/logos/ZktET_o_WU6Mm1sJzJLZhQ_48x48.png",
		},
		"KC": {
			Name:    "Chiefs",
			City:    "Kansas City",
			IconURL: "https://ssl.gstatic.com/onebox/media/sports/logos/5N0l1KbG1BHPyP8_S7SOXg_48x48.png",
		},
		"LV": {
			Name:    "Raiders",
			City:    "Las Vegas",
			IconURL: "https://ssl.gstatic.com/onebox/media/sports/logos/QysqoqJQsTbiJl8sPL12Yg_48x48.png",
		},
		"LAC": {
			Name:    "Chargers",
			City:    "Los Angeles",
			IconURL: "https://ssl.gstatic.com/onebox/media/sports/logos/EAQRZu91bwn1l8brW9HWBQ_48x48.png",
		},
		
		// NFC East
		"DAL": {
			Name:    "Cowboys",
			City:    "Dallas",
			IconURL: "https://ssl.gstatic.com/onebox/media/sports/logos/-zeHm0cuBjZXc2HRxRAI0g_48x48.png",
		},
		"NYG": {
			Name:    "Giants",
			City:    "New York",
			IconURL: "https://ssl.gstatic.com/onebox/media/sports/logos/q8qdTYh-OWR5uO_QZxFENw_48x48.png",
		},
		"PHI": {
			Name:    "Eagles",
			City:    "Philadelphia",
			IconURL: "https://ssl.gstatic.com/onebox/media/sports/logos/s4ab0JjXpDOespDSf9Z14Q_48x48.png",
		},
		"WAS": {
			Name:    "Commanders",
			City:    "Washington",
			IconURL: "https://ssl.gstatic.com/onebox/media/sports/logos/o0CCwss-QfFnJaVdGIHFmQ_48x48.png",
		},
		
		// NFC North
		"CHI": {
			Name:    "Bears",
			City:    "Chicago",
			IconURL: "https://ssl.gstatic.com/onebox/media/sports/logos/7uaGv3B13mXyBhHcTysHcA_48x48.png",
		},
		"DET": {
			Name:    "Lions",
			City:    "Detroit",
			IconURL: "https://ssl.gstatic.com/onebox/media/sports/logos/WE1l856fyyHh6eAbbb8hQQ_48x48.png",
		},
		"GB": {
			Name:    "Packers",
			City:    "Green Bay",
			IconURL: "https://ssl.gstatic.com/onebox/media/sports/logos/IlA4VGrUHzSVLCOcHsRKgg_48x48.png",
		},
		"MIN": {
			Name:    "Vikings",
			City:    "Minnesota",
			IconURL: "https://ssl.gstatic.com/onebox/media/sports/logos/Vmg4u0BSYZ-1Mc-5uyvxHg_48x48.png",
		},
		
		// NFC South
		"ATL": {
			Name:    "Falcons",
			City:    "Atlanta",
			IconURL: "https://ssl.gstatic.com/onebox/media/sports/logos/QNdwQPxtIRYUhnMBYq-bSA_48x48.png",
		},
		"CAR": {
			Name:    "Panthers",
			City:    "Carolina",
			IconURL: "https://ssl.gstatic.com/onebox/media/sports/logos/HsLg5tW_S7566EbsMPlcVQ_48x48.png",
		},
		"NO": {
			Name:    "Saints",
			City:    "New Orleans",
			IconURL: "https://ssl.gstatic.com/onebox/media/sports/logos/AC5-UEeN3V_fjkdFXtHWfQ_48x48.png",
		},
		"TB": {
			Name:    "Buccaneers",
			City:    "Tampa Bay",
			IconURL: "https://ssl.gstatic.com/onebox/media/sports/logos/efP_3b5BgkGE-HMCHx4huQ_48x48.png",
		},
		
		// NFC West
		"ARI": {
			Name:    "Cardinals",
			City:    "Arizona",
			IconURL: "https://ssl.gstatic.com/onebox/media/sports/logos/5Mh3xcc8uAsxAi3WZvfEyQ_48x48.png",
		},
		"LAR": {
			Name:    "Rams",
			City:    "Los Angeles",
			IconURL: "https://ssl.gstatic.com/onebox/media/sports/logos/CXW68CjwPIaUurbvSUSyJw_48x48.png",
		},
		"SF": {
			Name:    "49ers",
			City:    "San Francisco",
			IconURL: "https://ssl.gstatic.com/onebox/media/sports/logos/ku3s7M4k5KMagYcFTCie_g_48x48.png",
		},
		"SEA": {
			Name:    "Seahawks",
			City:    "Seattle",
			IconURL: "https://ssl.gstatic.com/onebox/media/sports/logos/iVPY42GLuHmD05DiOvNSVg_48x48.png",
		},
	}
}

// GetTeamIconURL returns the team icon URL for a given team abbreviation
func GetTeamIconURL(teamAbbr string) string {
	teams := GetTeamData()
	if team, exists := teams[teamAbbr]; exists {
		return team.IconURL
	}
	return "" // Return empty string if team not found
}

// GetTeamName returns the full team name for a given team abbreviation
func GetTeamName(teamAbbr string) string {
	teams := GetTeamData()
	if team, exists := teams[teamAbbr]; exists {
		return team.City + " " + team.Name
	}
	return teamAbbr // Return abbreviation if team not found
}