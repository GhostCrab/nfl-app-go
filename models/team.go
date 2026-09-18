package models

// Team represents an NFL team
type Team struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	City     string `json:"city"`
	Abbr     string `json:"abbr"`
	IconURL  string `json:"iconURL"`
	Active   bool   `json:"active"`
}

// String returns a string representation of the team
func (t *Team) String() string {
	return t.City + " " + t.Name
}

// DisplayName returns the full display name
func (t *Team) DisplayName() string {
	return t.City + " " + t.Name
}