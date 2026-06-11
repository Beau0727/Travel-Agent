package amap

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"
)

type CityInfo struct {
	Name      string
	Province  string
	Adcode    string
	Citycode  string
	Location  string
	Latitude  *float64
	Longitude *float64
}

type CityResolver struct {
	amap *Client
}

func NewCityResolver(amap *Client) *CityResolver {
	return &CityResolver{amap: amap}
}

func (r *CityResolver) Resolve(ctx context.Context, city string) (CityInfo, error) {
	city = strings.TrimSpace(city)
	if city == "" {
		return CityInfo{}, errors.New("city is empty")
	}
	if r == nil || r.amap == nil || !r.amap.Enabled() {
		return CityInfo{}, errors.New("amap client is disabled")
	}

	var raw struct {
		Status   string `json:"status"`
		Info     string `json:"info"`
		Infocode string `json:"infocode"`
		Count    string `json:"count"`
		Geocodes []struct {
			FormattedAddress string `json:"formatted_address"`
			Province         any    `json:"province"`
			City             any    `json:"city"`
			Adcode           string `json:"adcode"`
			Citycode         string `json:"citycode"`
			Location         string `json:"location"`
		} `json:"geocodes"`
	}

	values := url.Values{}
	values.Set("address", city)
	values.Set("city", city)

	if err := r.amap.GetV3(ctx, "/geocode/geo", values, &raw); err != nil {
		return CityInfo{}, err
	}
	if raw.Status != "1" || len(raw.Geocodes) == 0 {
		if raw.Info == "" {
			raw.Info = "no geocode result"
		}
		return CityInfo{}, errors.New(raw.Info)
	}

	item := raw.Geocodes[0]
	latitude, longitude := splitAmapLngLat(item.Location)
	name := stringFromAmapValue(item.City)
	if name == "" {
		name = city
	}

	return CityInfo{
		Name:      name,
		Province:  stringFromAmapValue(item.Province),
		Adcode:    item.Adcode,
		Citycode:  item.Citycode,
		Location:  item.Location,
		Latitude:  latitude,
		Longitude: longitude,
	}, nil
}

func splitAmapLngLat(location string) (*float64, *float64) {
	parts := strings.Split(location, ",")
	if len(parts) != 2 {
		return nil, nil
	}
	longitude, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	latitude, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err1 != nil || err2 != nil {
		return nil, nil
	}
	return &latitude, &longitude
}

func stringFromAmapValue(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
				parts = append(parts, strings.TrimSpace(text))
			}
		}
		return strings.Join(parts, " ")
	default:
		return ""
	}
}
