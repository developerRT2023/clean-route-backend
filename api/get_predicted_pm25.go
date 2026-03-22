package api

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/sadityakumar9211/clean-route-backend/models"
	"github.com/spf13/viper"
)

type Post struct {
	FPMVec []float64 `json:"fpm_vec"`
}

func GetPredictedPm25(df []models.FeatureVector) ([]float64, error) {

	var awsModelEndpoint string
	var awsModelEndpointError bool

	if os.Getenv("RAILWAY") == "true" {
		awsModelEndpoint = os.Getenv("AWS_MODEL_ENDPOINT")
	} else {
		awsModelEndpoint, awsModelEndpointError = viper.Get("AWS_MODEL_ENDPOINT").(string)
		if !awsModelEndpointError {
			log.Fatalf("Invalid type assertion")
		}
	}

	// fmt.Println("The Query url is: ", awsModelEndpoint)

	jsonData, err := json.Marshal(df)
	checkErrNil(err)

	r, err := http.NewRequest("POST", awsModelEndpoint, bytes.NewBuffer(jsonData))
	checkErrNil(err)

	r.Header.Add("Content-Type", "application/json")

	// fmt.Println("Just before making the request...")
	resp, err := httpClient.Do(r)
	checkErrNil(err)

	if resp.StatusCode != http.StatusOK {
		log.Fatal("The Amazon EC2 instance is not responding...Got status code: ", resp.StatusCode)
	}

	defer resp.Body.Close()
	// fmt.Println("After the Request...")
	post := &Post{}

	err = json.NewDecoder(resp.Body).Decode(post)
	checkErrNil(err)

	if resp.StatusCode != http.StatusOK {
		log.Fatal("Error while making calling API endpoint", err)
		return nil, err
	}

	// fmt.Println("++++++++++++++ Inside the get predicted pm2.5 ++++++++++++++++")
	// fmt.Println("Acutal: ", inputFeatures.IPM, "Predicted: ", post.FPM)
	return post.FPMVec, nil
}
