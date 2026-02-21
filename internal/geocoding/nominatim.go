package geocoding

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// Address contains full address details from Nominatim
type Address struct {
	Village      string `json:"village,omitempty"`
	CityDistrict string `json:"city_district,omitempty"`
	City         string `json:"city,omitempty"`
	State        string `json:"state,omitempty"`
	Region       string `json:"region,omitempty"`
	Postcode     string `json:"postcode,omitempty"`
	Country      string `json:"country,omitempty"`
	CountryCode  string `json:"country_code,omitempty"`
}

// LocationInfo represents reverse geocoded location data
type LocationInfo struct {
	DisplayName string  `json:"display_name"`
	Name        string  `json:"name"`
	Address     Address `json:"address"`
}

// NominatimResponse represents the API response
type NominatimResponse struct {
	DisplayName string `json:"display_name"`
	Name        string `json:"name"`
	Address     struct {
		Village       string `json:"village"`
		Neighbourhood string `json:"neighbourhood"`
		Town          string `json:"town"`
		CityDistrict  string `json:"city_district"`
		City          string `json:"city"`
		Municipality  string `json:"municipality"`
		County        string `json:"county"`
		State         string `json:"state"`
		Region        string `json:"region"`
		Postcode      string `json:"postcode"`
		Country       string `json:"country"`
		CountryCode   string `json:"country_code"`
	} `json:"address"`
}

// NominatimClient handles reverse geocoding requests
type NominatimClient struct {
	httpClient  *http.Client
	baseURL     string
	userAgent   string
	rateLimiter *time.Ticker
}

// NewNominatimClient creates a new Nominatim client
// Rate limited to 1 request per second per Nominatim usage policy
func NewNominatimClient(userAgent string) *NominatimClient {
	return &NominatimClient{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		baseURL:     "https://nominatim.openstreetmap.org/reverse",
		userAgent:   userAgent,
		rateLimiter: time.NewTicker(1100 * time.Millisecond), // slightly over 1 second
	}
}

// ReverseGeocode converts coordinates to location info
func (c *NominatimClient) ReverseGeocode(ctx context.Context, lat, lng float64) (*LocationInfo, error) {
	// Rate limit - wait for ticker or ctx cancellation
	// Removes the global mutex to prevent goroutine starvation on high concurrency
	select {
	case <-c.rateLimiter.C:
		// Rate limit passed, proceed
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Build URL with zoom=15 for neighborhood-level detail
	params := url.Values{}
	params.Set("format", "jsonv2")
	params.Set("lat", fmt.Sprintf("%f", lat))
	params.Set("lon", fmt.Sprintf("%f", lng))
	params.Set("zoom", "15")
	params.Set("addressdetails", "1")

	reqURL := fmt.Sprintf("%s?%s", c.baseURL, params.Encode())

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Nominatim requires a valid User-Agent
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call Nominatim: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Nominatim returned status %d", resp.StatusCode)
	}

	var nominatimResp NominatimResponse
	if err := json.NewDecoder(resp.Body).Decode(&nominatimResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Extract best available village name
	village := nominatimResp.Address.Village
	if village == "" {
		village = nominatimResp.Address.Neighbourhood
	}

	// Extract best available city name
	city := nominatimResp.Address.City
	if city == "" {
		city = nominatimResp.Address.Town
	}
	if city == "" {
		city = nominatimResp.Address.Municipality
	}

	return &LocationInfo{
		DisplayName: nominatimResp.DisplayName,
		Name:        nominatimResp.Name,
		Address: Address{
			Village:      village,
			CityDistrict: nominatimResp.Address.CityDistrict,
			City:         city,
			State:        nominatimResp.Address.State,
			Region:       nominatimResp.Address.Region,
			Postcode:     nominatimResp.Address.Postcode,
			Country:      nominatimResp.Address.Country,
			CountryCode:  nominatimResp.Address.CountryCode,
		},
	}, nil
}
