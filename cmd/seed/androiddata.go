package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"time"
)

// Data dummy app Android, disalin oleh scripts/extract_android_seed.py dari Fake*Repository (jangan diedit tangan).
//
//go:embed data/*.json
var dataFS embed.FS

//go:embed img/*.jpg
var imgFS embed.FS

// rel adalah waktu relatif terhadap saat seed dijalankan, dalam menit ({"min": -55}).
type rel struct {
	Min float64 `json:"min"`
}

func (r rel) at(now time.Time) time.Time { return now.Add(time.Duration(r.Min * float64(time.Minute))) }

type aMerchant struct {
	ID, Name, Status, Location, Banner, About, Address, Halal string
	OpenHours                                                 string   `json:"open_hours"`
	Verified                                                  bool     `json:"verified"`
	Rating                                                    *float64 `json:"rating"`
	RatingCount                                               int      `json:"rating_count"`
	Photos                                                    []string `json:"photos"`
	Allergens                                                 []string `json:"allergens"`
	TodaysOffer                                               *string  `json:"todays_offer"`
	DeliveryAvailable                                         bool     `json:"delivery_available"`
	PrepMinutes                                               int      `json:"prep_minutes"`
	DistanceKm                                                float64  `json:"distance_km"`
	TotalRescued                                              int      `json:"total_rescued"`
}

type aListing struct {
	ID            string
	MerchantID    string `json:"merchant_id"`
	Title, Tier   string
	Original      *int `json:"original"`
	Price         *int `json:"price"`
	IsFree        bool `json:"is_free"`
	Image         string
	Gallery       []string
	MadeHoursAgo  float64 `json:"made_hours_ago"`
	Storage       string
	PickupEndInH  float64 `json:"pickup_end_in_h"`
	PickupStartIn float64 `json:"pickup_start_in_h"`
	Description   string
	Stock         int
	Allergens     []string
	Sold          int
	Halal         string
	SafetyChecks  []string `json:"safety_checks"`
}

type aReview struct {
	ID         string
	MerchantID string `json:"merchant_id"`
	Author     string
	Stars      int
	Comment    string
	At         rel
	Photos     []string
	ItemTitle  string `json:"item_title"`
	Tags       []string
}

type aVoucher struct {
	Code, Title, Description string
	PercentOff               *int `json:"percent_off"`
	FlatOff                  *int `json:"flat_off"`
	MaxDiscount              *int `json:"max_discount"`
	MinSpend                 int  `json:"min_spend"`
	Expires                  rel
	Image                    string
	Accent                   int64
	Terms                    []string
	AppliesTo                string `json:"applies_to"`
}

type aNotification struct {
	ID, Type, Title, Body string
	At                    rel
	IsRead                bool `json:"is_read"`
}

type aChat struct {
	ID         string
	MerchantID string `json:"merchant_id"`
	FromBuyer  bool   `json:"from_buyer"`
	Text       string
	At         rel
	ListingID  *string `json:"listing_id"`
	IsRead     bool    `json:"is_read"`
	Buyer      string
}

type aOrderLine struct {
	ListingID         string `json:"listing_id"`
	Title, Tier       string
	Image             string
	Quantity          int
	UnitPrice         int `json:"unit_price"`
	UnitOriginalPrice int `json:"unit_original_price"`
}

type aOrder struct {
	ID, Code, Payment, Note, Status string
	MerchantID                      string `json:"merchant_id"`
	Lines                           []aOrderLine
	Created                         rel
	PickupEnd                       rel  `json:"pickup_end"`
	Completed                       *rel `json:"completed"`
	Rating                          *struct {
		Stars   int
		Tags    []string
		Comment string
	}
}

type aIncoming struct {
	ID, Buyer, Code string
	ItemTitle       string `json:"item_title"`
	Quantity, Total int
	Unit            string
	Fulfillment     string
	CourierLabel    *string `json:"courier_label"`
	Created         rel
	PickupBy        rel `json:"pickup_by"`
	Status          string
}

type aAddress struct {
	ID, Label string
	FullAddr  string `json:"full_address"`
	Lat, Lng  float64
	Note      string
}

type aPayment struct{ ID, Kind, Label, Detail string }

type aProfile struct {
	Name, Email, Phone, Location string
	MemberSince                  string `json:"member_since"`
	Impact                       struct {
		Portions  int `json:"portions"`
		CompostKg int `json:"compost_kg"`
		CarbonKg  int `json:"carbon_kg"`
		Saved     int `json:"saved_rupiah"`
	}
}

type androidData struct {
	Merchants     []aMerchant
	Listings      []aListing
	Reviews       []aReview
	Vouchers      []aVoucher
	Claimed       []string
	Notifications []aNotification
	Chat          []aChat
	Orders        []aOrder
	Incoming      []aIncoming
	Addresses     []aAddress
	Payments      []aPayment
	Saved         []string
	Profile       aProfile
	Images        []string
}

func loadAndroidData() (*androidData, error) {
	d := &androidData{}
	for name, dst := range map[string]any{"merchants": &d.Merchants, "listings": &d.Listings, "reviews": &d.Reviews, "vouchers": &d.Vouchers,
		"claimed_vouchers": &d.Claimed, "notifications": &d.Notifications, "chat": &d.Chat, "orders": &d.Orders, "incoming": &d.Incoming,
		"addresses": &d.Addresses, "payment_methods": &d.Payments, "saved": &d.Saved, "profile": &d.Profile, "images": &d.Images} {
		b, err := dataFS.ReadFile("data/" + name + ".json")
		if err != nil {
			return nil, fmt.Errorf("baca data/%s.json: %w (jalankan make sync-android-seed)", name, err)
		}
		if err := json.Unmarshal(b, dst); err != nil {
			return nil, fmt.Errorf("parse data/%s.json: %w", name, err)
		}
	}
	return d, nil
}
