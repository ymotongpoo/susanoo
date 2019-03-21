// Copyright 2019 Yoshi Yamaguchi
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	owm "github.com/briandowns/openweathermap"
)

const (
	// Interval time period to fetch data from OpenWeatherMap.
	// Free tier updates API data in 2 hours or less time interval.
	// ref: https://openweathermap.org/price
	OWMPollInterval = 15 * time.Second

	// https://darksky.net/dev/docs#forecast-request
	DarkSkyForecastAPIURL = "https://api.darksky.net/forecast/%s/%f,%f?exclude=minutely,hourly,daily,alerts&lang=en&units=si"

	// DarkSky has limit of 1000 call per dar for free tier.
	// https://darksky.net/dev/docs/faq#cost
	DarkSkyPollInterval = 90 * time.Second
)

var (
	// Using Makefile in the repo embeds OWM_API_KEY on build.
	OWMAPIKey string

	// Using Makefile in the repo embds DARK_SKY_API_KEY on build.
	DarkSkyAPIKey string

	// Shibuya, Tokyo, Japan
	TargetCityLatitude  = 35.6620
	TargetCityLongitude = 139.7038

	// Convert TargetCityLatitude and TargetCityLongitude to owm.Corrdinates.
	TargetCoodinates *owm.Coordinates = &owm.Coordinates{
		Longitude: TargetCityLongitude,
		Latitude:  TargetCityLatitude,
	}
)

type Weather struct {
	Temperature float64
	Pressure    float64
	Humidity    int
	Weather     string
	WindSpeed   float64
	WindDeg     float64
	Cloudiness  int
	Rainfall    float64
	Snowfall    float64
	UV          float64
}

func main() {
	owmw, owmuv := InitOpenWeatherMap()

	owmTicker := time.NewTicker(OWMPollInterval)
	dsTicker := time.NewTicker(DarkSkyPollInterval)
	for {
		select {
		case <-owmTicker.C:
			owmw.CurrentByCoordinates(TargetCoodinates)
			w := OWMToWeather(owmw, owmuv)
			fmt.Printf("OWM: Weather: %s, Temp: %f, Pressure: %f, Humidity: %d, UV: %f\n", w.Weather, w.Temperature, w.Pressure, w.Humidity, w.UV)
		case <-dsTicker.C:
			f := CallDarkSkyForecast()
			w := DSToWeather(f)
			fmt.Printf("DS: Weather: %s, Temp: %f, Pressure: %f, Humidity: %d, UV: %f\n", w.Weather, w.Temperature, w.Pressure, w.Humidity, w.UV)
		}
	}
}

func InitOpenWeatherMap() (*owm.CurrentWeatherData, *owm.UV) {
	w, err := owm.NewCurrent("C", "EN", OWMAPIKey)
	if err != nil {
		log.Fatalf("failed to initialize OpenWeatherMap current weather data: %s", err)
	}
	w.CurrentByCoordinates(TargetCoodinates)

	uv, err := owm.NewUV(OWMAPIKey)
	if err != nil {
		log.Fatalf("failed to initialize OpenWeatherMap UV data: %s", err)
	}
	uv.Current(TargetCoodinates)
	return w, uv
}

type DarkSkyForecast struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	TimeZone  string  `json:"timezone"`
	Currently struct {
		Time            int64   `json:"time"`
		Summary         string  `json:"summary"`
		Icon            string  `json:"icon"`
		Temperature     float64 `json:"temperature"`
		Pressure        float64 `json:"pressure"`
		Humidity        float64 `json:"humidity"`
		WindSpeed       float64 `json:"windSpeed"`
		WindBearing     int     `json:"windBearing"`
		PrecipIntensity float64 `json:"precipIntensity"`
		CloudCover      float64 `json:"cloudCover"`
		UVIndex         float64 `json:"uvIndex"`
	} `json:"currently"`
}

func CallDarkSkyForecast() *DarkSkyForecast {
	resp, err := http.Get(
		fmt.Sprintf(DarkSkyForecastAPIURL, DarkSkyAPIKey, TargetCityLatitude, TargetCityLongitude))
	if err != nil {
		log.Fatalf("failed to call DarkSky forecast API: %s", err)
	}
	defer resp.Body.Close()

	decoder := json.NewDecoder(resp.Body)
	var f DarkSkyForecast
	err = decoder.Decode(&f)
	if err != nil {
		log.Fatalf("failed to decode DarkSky reponse: %s", err)
	}
	return &f
}

func OWMToWeather(w *owm.CurrentWeatherData, uv *owm.UV) *Weather {
	return &Weather{
		Temperature: w.Main.Temp,
		Pressure:    w.Main.Pressure,
		Humidity:    w.Main.Humidity,
		Weather:     w.Weather[0].Main,
		WindSpeed:   w.Wind.Speed,
		WindDeg:     w.Wind.Deg,
		Cloudiness:  w.Clouds.All,
		Rainfall:    w.Rain.ThreeH / 3,
		UV:          uv.Value,
	}
}

func DSToWeather(f *DarkSkyForecast) *Weather {
	return &Weather{
		Temperature: f.Currently.Temperature,
		Pressure:    f.Currently.Pressure,
		Humidity:    int(f.Currently.Humidity * 100),
		Weather:     f.Currently.Summary,
		WindSpeed:   f.Currently.WindSpeed,
		WindDeg:     float64(f.Currently.WindBearing),
		Cloudiness:  int(f.Currently.CloudCover * 100),
		Rainfall:    f.Currently.PrecipIntensity,
	}
}
