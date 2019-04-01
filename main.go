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
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"go.uber.org/zap/zapcore"

	"contrib.go.opencensus.io/exporter/stackdriver"
	owm "github.com/briandowns/openweathermap"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	"go.uber.org/zap"
)

const (
	// OCReportInterval is the interval for OpenCensus to send stats data to
	// Stackdriver Monitoring via its exporter.
	// NOTE: this value should not be no less than 1 minute. Detailes are in the doc.
	// https://cloud.google.com/monitoring/custom-metrics/creating-metrics#writing-ts
	OCReportInterval = 60 * time.Second

	// Measure namess for respecitive OpenCensus Measure
	MeasureTemperature = "temperature"
	MeasurePressure    = "pressure"
	MeasureHumidity    = "humidity"
	MeasureWindSpeed   = "windspeed"
	MeasureWindDeg     = "winddeg"

	// Units are used to define Measures of OpenCensus.
	TemperatureUnit = "C"
	PressureUnit    = "hPa"
	HumidityUnit    = "%"
	WindSpeedUnit   = "mps"
	WindDegUnit     = "degree"

	// ResouceNamespace is used for the exporter to have resource labels.
	ResourceNamespace = "ymotongpoo"
)

var (
	// Measure variables
	MTemperature = stats.Float64(MeasureTemperature, "air temperature", TemperatureUnit)
	MPressure    = stats.Float64(MeasurePressure, "barometric pressure", PressureUnit)
	MHumidity    = stats.Int64(MeasureHumidity, "air humidity", HumidityUnit)
	MWindSpeed   = stats.Float64(MeasureWindSpeed, "wind speed", WindSpeedUnit)
	MWindDeg     = stats.Float64(MeasureWindDeg, "wind degree from North", WindDegUnit)

	TemperatureView = &view.View{
		Name:        MeasureTemperature,
		Measure:     MTemperature,
		Description: "air temperature",
		Aggregation: view.LastValue(),
	}

	PressureView = &view.View{
		Name:        MeasurePressure,
		Measure:     MPressure,
		Description: "barometric pressure",
		Aggregation: view.LastValue(),
	}

	HumidityView = &view.View{
		Name:        MeasureHumidity,
		Measure:     MHumidity,
		Description: "air humidity",
		Aggregation: view.LastValue(),
	}

	WindSpeedView = &view.View{
		Name:        MeasureWindSpeed,
		Measure:     MWindSpeed,
		Description: "wind speed",
		Aggregation: view.LastValue(),
	}

	WeatherReportViews = []*view.View{
		TemperatureView,
		PressureView,
		//HumidityView,
		WindSpeedView,
	}

	// KeyNodeId is the key for label in "generic_node",
	KeyNodeId, _ = tag.NewKey("node_id")
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

	logger *zap.SugaredLogger
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

func init() {
	cfg := zap.Config{
		Encoding:         "json",
		Level:            zap.NewAtomicLevelAt(zapcore.DebugLevel),
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
		EncoderConfig: zapcore.EncoderConfig{
			MessageKey:     "message",
			LevelKey:       "severity",
			EncodeLevel:    zapcore.LowercaseLevelEncoder,
			TimeKey:        "timestamp",
			EncodeTime:     zapcore.ISO8601TimeEncoder,
			EncodeDuration: zapcore.StringDurationEncoder,
			CallerKey:      "caller",
			EncodeCaller:   zapcore.ShortCallerEncoder,
		},
	}

	l, err := cfg.Build()
	if err != nil {
		log.Fatalf("failed to create logger: %v", err)
	}
	logger = l.Sugar()
}

func main() {
	owmw, owmuv, err := InitOpenWeatherMap()
	if err != nil {
		logger.Fatalf("failed to initialize OpenWeatherMap: %v", err)
	}

	exporter := InitExporter()
	defer exporter.Flush()
	InitOpenCensusStats(exporter)

	owmTicker := time.NewTicker(OWMPollInterval)
	dsTicker := time.NewTicker(DarkSkyPollInterval)
	for {
		select {
		case <-owmTicker.C:
			if err := owmw.CurrentByCoordinates(TargetCoodinates); err != nil {
				logger.Errorf("failed to call current data from OpenWeatherMap: %v", err)
				break
			}
			w := OWMToWeather(owmw, owmuv)
			if err := RecordMeasurement("openweathermap", w); err != nil {
				logger.Errorf("failed to record: %v", err)
			}
		case <-dsTicker.C:
			f, err := CallDarkSkyForecast()
			if err != nil {
				logger.Errorf("failed to call DarkSky: %v", err)
				break
			}
			w := DSToWeather(f)
			if err := RecordMeasurement("darksky", w); err != nil {
				logger.Errorf("failed to record: %v", err)
			}
		}
	}
}

type GenericNodeMonitoredResource struct{}

func (mr *GenericNodeMonitoredResource) MonitoredResource() (string, map[string]string) {
	labels := map[string]string{
		"location":  "asia-northeast1-a",
		"namespace": "ymotongpoo",
		"node_id":   "public-data",
	}
	return "generic_node", labels
}

func GetMetricType(v *view.View) string {
	return fmt.Sprintf("custom.googleapis.com/%s", v.Name)
}

func InitExporter() *stackdriver.Exporter {
	labels := &stackdriver.Labels{}
	labels.Set("source", "weather-api", "")
	exporter, err := stackdriver.NewExporter(stackdriver.Options{
		ProjectID:               os.Getenv("GOOGLE_CLOUD_PROJECT"),
		Location:                "asia-northeast1-a",
		MonitoredResource:       &GenericNodeMonitoredResource{},
		DefaultMonitoringLabels: labels,
		GetMetricType:           GetMetricType,
	})
	if err != nil {
		log.Fatal("failed to initialize ")
	}
	return exporter
}

func InitOpenCensusStats(exporter *stackdriver.Exporter) {
	view.SetReportingPeriod(OCReportInterval)
	view.RegisterExporter(exporter)
	view.Register(WeatherReportViews...)
}

func RecordMeasurement(id string, w *Weather) error {
	ctx, err := tag.New(context.Background(), tag.Upsert(KeyNodeId, id))
	if err != nil {
		logger.Errorf("failed to insert key: %v", err)
		return err
	}

	stats.Record(ctx,
		MTemperature.M(w.Temperature),
		MPressure.M(w.Pressure),
		MHumidity.M(int64(w.Humidity)),
		MWindSpeed.M(w.WindSpeed),
	)
	return nil
}

func InitOpenWeatherMap() (*owm.CurrentWeatherData, *owm.UV, error) {
	w, err := owm.NewCurrent("C", "EN", OWMAPIKey)
	if err != nil {
		logger.Errorf("failed to initialize OpenWeatherMap current weather data: %v", err)
		return nil, nil, err
	}
	w.CurrentByCoordinates(TargetCoodinates)

	uv, err := owm.NewUV(OWMAPIKey)
	if err != nil {
		logger.Errorf("failed to initialize OpenWeatherMap UV data: %s", err)
		return nil, nil, err
	}
	uv.Current(TargetCoodinates)
	return w, uv, nil
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

func CallDarkSkyForecast() (*DarkSkyForecast, error) {
	resp, err := http.Get(
		fmt.Sprintf(DarkSkyForecastAPIURL, DarkSkyAPIKey, TargetCityLatitude, TargetCityLongitude))
	if err != nil {
		logger.Errorf("failed to call DarkSky forecast API: %s", err)
		return nil, err
	}
	defer resp.Body.Close()

	decoder := json.NewDecoder(resp.Body)
	var f DarkSkyForecast
	err = decoder.Decode(&f)
	if err != nil {
		logger.Errorf("failed to decode DarkSky reponse: %s", err)
		return nil, err
	}
	return &f, nil
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
