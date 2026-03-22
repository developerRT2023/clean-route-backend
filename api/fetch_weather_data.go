package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/url"

	"github.com/sadityakumar9211/clean-route-backend/models/openweather"
	"github.com/spf13/viper"
)

func FetchWeatherData(location []float64) openweather.WeatherData {
	baseUrl := "https://api.openweathermap.org/data/3.0/onecall?"

	openWeatherAccessToken, openWeatherAccessTokenError := viper.Get("OPEN_WEATHER_API_KEY").(string)
	if !openWeatherAccessTokenError {
		log.Fatalf("Invalid type assertion")
	}

	weatherParams := url.Values{}
	weatherParams.Add("lat", fmt.Sprintf("%f", location[1]))
	weatherParams.Add("lon", fmt.Sprintf("%f", location[0]))
	weatherParams.Add("exclude", "alerts,daily")
	weatherParams.Add("units", "metric")
	weatherParams.Add("appid", openWeatherAccessToken)


	weatherUrl := baseUrl + weatherParams.Encode()

	// fmt.Println("The Query url is: ", weatherUrl)

	resp, err := httpClient.Get(weatherUrl)
	checkErrNil(err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	checkErrNil(err)

	var weatherResponse openweather.WeatherData

	err = json.Unmarshal([]byte(body), &weatherResponse)
	if err != nil {
		log.Fatal("Error while unmarshling JSON: ", err)
	}

	return weatherResponse
}
