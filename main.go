// There is no car mode input -- it is by default driving-traffic

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	graphhopper "github.com/sadityakumar9211/clean-route-backend/models/graphhopper"
	mapbox "github.com/sadityakumar9211/clean-route-backend/models/mapbox"
	"github.com/sadityakumar9211/clean-route-backend/utils"
	"github.com/spf13/viper"
)

type formData struct {
	Source          [2]float64 `json:"source"`
	Destination     [2]float64 `json:"destination"`
	DelayCode       uint8      `json:"delayCode"`
	Mode            string     `json:"mode"`
	RoutePreference string     `json:"route_preference,omitempty"`
}

func findMapboxRoute(source [2]float64, destination [2]float64, delayCode uint8) mapbox.RouteData {
	baseUrl := "https://api.mapbox.com/directions/v5/mapbox/driving-traffic/" + fmt.Sprintf("%f,%f;%f,%f", source[0], source[1], destination[0], destination[1])

	var mapboxAccessToken string
	var mapboxAccessTokenError bool
	if os.Getenv("RAILWAY") == "true" {
		mapboxAccessToken = os.Getenv("MAPBOX_API_KEY")
	} else {
		mapboxAccessToken, mapboxAccessTokenError = viper.Get("MAPBOX_API_KEY").(string)
		if !mapboxAccessTokenError {
			log.Fatalf("Invalid type assertion")
		}
	}

	localTime := time.Now()
	departureTime := localTime.Add(30 * time.Duration(delayCode) * time.Minute).Format("2006-01-02T15:04")

	params := url.Values{}
	params.Add("steps", "true")
	params.Add("geometries", "geojson")
	params.Add("alternatives", "true")
	params.Add("waypoints_per_route", "true")
	params.Add("access_token", mapboxAccessToken)
	params.Add("depart_at", departureTime)

	url := baseUrl + "?" + params.Encode()

	resp, err := http.Get(url)
	checkErrNil(err)

	if resp.StatusCode != http.StatusOK {
		log.Fatal("Error while calling Mapbox API", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	checkErrNil(err)

	var routes mapbox.RouteData

	err = json.Unmarshal([]byte(body), &routes)
	if err != nil {
		log.Fatal("Error while unmarshling JSON: ", err)
	}

	fmt.Println("Distance", routes.Routes[0].Distance)
	fmt.Println("Total Exposure: ", routes.Routes[0].TotalExposure)
	fmt.Println("Total Energy: ", routes.Routes[0].TotalEnergy)
	fmt.Println("Code:", routes.Code)
	fmt.Println("UUID:", routes.UUID)
	return routes
}

func findGraphhopperRoute(source [2]float64, destination [2]float64, mode string) graphhopper.RouteData {
	baseUrl := "https://graphhopper.com/api/1/route?"

	var graphhopperApikey string
	var graphhopperApikeyError bool

	if os.Getenv("RAILWAY") == "true" {
		graphhopperApikey = os.Getenv("GRAPHHOPPER_API_KEY")
	} else {
		graphhopperApikey, graphhopperApikeyError = viper.Get("GRAPHHOPPER_API_KEY").(string)
		if !graphhopperApikeyError {
			log.Fatal("Found GraphHopper API key: ", graphhopperApikeyError)
		}
	}

	params := url.Values{}
	params.Add("point", fmt.Sprintf("%f,%f", source[1], source[0]))
	params.Add("point", fmt.Sprintf("%f,%f", destination[1], destination[0]))
	params.Add("vehicle", mode)
	params.Add("debug", "true")
	params.Add("key", graphhopperApikey)
	params.Add("type", "json")
	params.Add("points_encoded", "false")
	params.Add("algorithm", "alternative_route")
	params.Add("alternative_route.max_paths", "4")
	params.Add("alternative_route.max_weight_factor", "1.4")
	params.Add("alternative_route.max_share_factor", "0.6")
	params.Add("elevation", "true")

	url := baseUrl + params.Encode()

	resp, err := http.Get(url)
	checkErrNil(err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	checkErrNil(err)

	var routes graphhopper.RouteData

	err = json.Unmarshal([]byte(body), &routes)
	if err != nil {
		log.Fatalf("Error while unmarshling JSON: %s", err)
	}

	if len(routes.Paths) == 0 {
		return routes
	}

	// Access and work with the unmarshaled data
	fmt.Println("Total Paths: ", len(routes.Paths))
	fmt.Println("Distance:", routes.Paths[0].Distance)
	fmt.Println("UUID:", routes.Paths[0].Time)
	return routes
}

func findRoute(c *gin.Context) {
	var queryData formData
	if err := c.BindJSON(&queryData); err != nil {
		log.Fatal("Error while unmarshling JSON", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	fmt.Println(queryData)

	source := queryData.Source
	destination := queryData.Destination
	delayCode := queryData.DelayCode
	mode := queryData.Mode
	routePref := queryData.RoutePreference

	if mode == "driving-traffic" && (routePref == "fastest" || routePref == "balanced") {
		fmt.Println("@@@@@@@@@@@@@@@@@@@@Mapbox Route@@@@@@@@@@@@@@")
		// Calling Mapbox API
		var routes mapbox.RouteData = findMapboxRoute(source, destination, delayCode)
		// Finding the energy of each routes and exposure
		if mode == "driving-traffic" {
			mode = "car"
		}
		var energy_route graphhopper.RouteData = findGraphhopperRoute(source, destination, mode)

		// aligning the routes based on distance measure
		sort.SliceStable(energy_route.Paths, func(i, j int) bool {
			return energy_route.Paths[i].Distance < energy_route.Paths[j].Distance
		})

		sort.SliceStable(routes.Routes, func(i, j int) bool {
			return routes.Routes[i].Distance < routes.Routes[j].Distance
		})

		// Exposure and Energy
		for i := 0; i < len(routes.Routes) && i < len(energy_route.Paths); i++ {
			routes.Routes[i] = utils.CalculateRouteExposureMapbox(routes.Routes[i], delayCode)
			routes.Routes[i].Duration *= 1000
			routes.Routes[i].TotalEnergy = utils.CalculateRouteEnergy(energy_route.Paths[i], mode)
			fmt.Println("Total Energy: ", routes.Routes[i].TotalEnergy)
			fmt.Println("Distance: ", routes.Routes[i].Distance)
			fmt.Println("Duration: ", routes.Routes[i].Duration)
		}
		// fmt.Println("I was here...")
		// Perform calculations and return the best path
		if routePref == "fastest" {
			index := 0
			for i := 0; i < len(routes.Routes); i++ {
				if routes.Routes[i].Duration < routes.Routes[index].Duration {
					index = i
				}
			}

			c.IndentedJSON(http.StatusOK, routes.Routes[index]) // returning the fastest route
			return
		} else if routePref == "balanced" {
			// If we have only one path in the result of API call
			if len(routes.Routes) == 1 {
				c.IndentedJSON(http.StatusOK, routes.Routes[0])
				return
			}

			// If we have two paths (max paths in case of mapbox)
			if len(routes.Routes) == 2 {
				if routes.Routes[0].Duration-routes.Routes[1].Duration < 5*60*1000 && routes.Routes[0].Distance-routes.Routes[1].Distance < 500 {
					// return path with least exposure
					if routes.Routes[0].TotalExposure < routes.Routes[1].TotalExposure {
						c.IndentedJSON(http.StatusOK, routes.Routes[0])
					} else {
						c.IndentedJSON(http.StatusOK, routes.Routes[1])
					}
				} else {
					if routes.Routes[0].Duration < routes.Routes[1].Duration {
						c.IndentedJSON(http.StatusOK, routes.Routes[0])
					} else {
						c.IndentedJSON(http.StatusOK, routes.Routes[1])
					}
				}
				return
			}
		}
		c.IndentedJSON(http.StatusBadRequest, "Error: Incorrect route preference suspected.")
	} else {
		fmt.Println("@@@@@@@@@@@@@@@@Graphhopper Route@@@@@@@@@@@@@@@@@@@@@@@")
		// Calling GraphHopper API
		if mode == "driving-traffic" {
			mode = "car"
		}
		var routes graphhopper.RouteData = findGraphhopperRoute(source, destination, mode)
		for i := 0; i < len(routes.Paths); i++ {
			routes.Paths[i] = utils.CalculateRouteExposureGraphhopper(routes.Paths[i], delayCode)
			routes.Paths[i].TotalEnergy = utils.CalculateRouteEnergy(routes.Paths[i], mode)
		}

		if mode == "car" {
			mode = "driving-traffic"
		}

		// Perform calculation and return the best path
		if len(routes.Paths) == 0 {
			c.IndentedJSON(http.StatusBadRequest, "Error: No routes returned by routing service.")
			return
		}
		if routePref == "shortest" {
			// sort the routes with distance and return the shortest path.
			sort.SliceStable(routes.Paths, func(i, j int) bool {
				return routes.Paths[i].Distance < routes.Paths[j].Distance
			})
			// fmt.Println(routes)
			c.IndentedJSON(http.StatusOK, routes.Paths[0])
			return
		} else if routePref == "fastest" {
			// sort the routes with time and return the fastest path
			sort.SliceStable(routes.Paths, func(i, j int) bool {
				return routes.Paths[i].Time < routes.Paths[j].Time
			})
			c.IndentedJSON(http.StatusOK, routes.Paths[0])
			return
		} else if routePref == "leap" {
			// sort the routes with exposure and return the leap path
			sort.SliceStable(routes.Paths, func(i, j int) bool {
				return routes.Paths[i].TotalExposure < routes.Paths[j].TotalExposure
			})
			// fmt.Println(routes)
			c.IndentedJSON(http.StatusOK, routes.Paths[0])
			return
		} else if routePref == "emission" {
			// sort the routes with total energy and return the lco2 path
			sort.SliceStable(routes.Paths, func(i, j int) bool {
				return routes.Paths[i].TotalEnergy < routes.Paths[j].TotalEnergy
			})
			c.IndentedJSON(http.StatusOK, routes.Paths[0])
			return
		} else if routePref == "balanced" {
			// check it out how it is implemented...
			// First find the two fastest paths and then find the path with the smallest exposure

			// If we have only one path in the result of API call
			if len(routes.Paths) == 1 {
				c.IndentedJSON(http.StatusOK, routes.Paths[0])
				return
			} else if len(routes.Paths) == 2 {
				if routes.Paths[0].Time-routes.Paths[1].Time < 5*60*1000 && routes.Paths[0].Distance-routes.Paths[1].Distance < 500 {
					// return path with least exposure
					if routes.Paths[0].TotalExposure < routes.Paths[1].TotalExposure {
						c.IndentedJSON(http.StatusOK, routes.Paths[0])
					} else {
						c.IndentedJSON(http.StatusOK, routes.Paths[1])
					}
				} else {
					if routes.Paths[0].Time < routes.Paths[1].Time {
						c.IndentedJSON(http.StatusOK, routes.Paths[0])
					} else {
						c.IndentedJSON(http.StatusOK, routes.Paths[1])
					}
				}

				return
			} else {
				// Cap slice bounds to the actual number of available paths
				topN := len(routes.Paths)
				if topN > 3 {
					topN = 3
				}

				// sorting the top three routes based on exposure
				sort.Slice(routes.Paths, func(i, j int) bool {
					return routes.Paths[i].TotalExposure < routes.Paths[j].TotalExposure
				})

				// sorting top routes (up to 3) based on time
				sort.SliceStable(routes.Paths[:topN], func(i, j int) bool {
					return routes.Paths[i].Time < routes.Paths[j].Time
				})

				// sorting the top two balanced(time, exposure) routes with energy
				topTwo := topN
				if topTwo > 2 {
					topTwo = 2
				}
				sort.Slice(routes.Paths[:topTwo], func(i, j int) bool {
					return routes.Paths[i].TotalEnergy < routes.Paths[j].TotalEnergy
				})
			}

			c.IndentedJSON(http.StatusOK, routes.Paths[0])
			return
		}
		c.IndentedJSON(http.StatusBadRequest, "Error: Incorrect route preference suspected.")
	}
}

//

func findAllRoutes(c *gin.Context) {
	// This is where we will find all the routes and return the result.
	fmt.Println("################ Inside Get All Routes ############")
	var queryData formData
	if err := c.BindJSON(&queryData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	fmt.Println(queryData)

	source := queryData.Source
	destination := queryData.Destination
	delayCode := queryData.DelayCode
	mode := queryData.Mode
	routePref := queryData.RoutePreference

	// find the mapbox and graphhopper routes and then based on the mode and routePref sort and select the best route and return the route.
	// But there is only mode and the routePref is all (i.e.)
	// Mode: Car and Motorbike

	// for car - fastest and balanced, I will need the mapbox routes

	// in all other cases, I will need both mapbox and graphhopper routes

	if mode == "scooter" {
		// find the graphhopper route
		fmt.Println("@@@@@@@@@@@@@ Finding the route for Motorbike @@@@@@@@@@@@@@@@")
		var routes graphhopper.RouteData = findGraphhopperRoute(source, destination, mode)
		for i := 0; i < len(routes.Paths); i++ {
			routes.Paths[i] = utils.CalculateRouteExposureGraphhopper(routes.Paths[i], delayCode)
			routes.Paths[i].TotalEnergy = utils.CalculateRouteEnergy(routes.Paths[i], mode)
			fmt.Println("Distance: ", routes.Paths[i].Distance)
			fmt.Println("Duration: ", routes.Paths[i].Time)
			fmt.Println("Total Exposure: ", routes.Paths[i].TotalExposure)
			fmt.Println("Total Energy: ", routes.Paths[i].TotalEnergy)
		}

		var routeList graphhopper.RouteList

		routeList.Source = source[:]
		routeList.Destination = destination[:]
		routeList.DelayCode = delayCode
		routeList.Mode = mode
		routeList.RoutePref = routePref

		// for fastest route
		var index int
		index = 0
		for i := 0; i < len(routes.Paths); i++ {
			if routes.Paths[i].Time < routes.Paths[index].Time {
				index = i
			}
		}
		routeList.Fastest = routes.Paths[index]

		// for shortest route
		index = 0
		for i := 0; i < len(routes.Paths); i++ {
			if routes.Paths[i].Distance < routes.Paths[index].Distance {
				index = i
			}
		}
		routeList.Shortest = routes.Paths[index]

		// for leap route
		index = 0
		for i := 0; i < len(routes.Paths); i++ {
			if routes.Paths[i].TotalExposure < routes.Paths[index].TotalExposure {
				index = i
			}
		}
		routeList.LeapG = routes.Paths[index]

		// for lco2 route
		index = 0
		for i := 0; i < len(routes.Paths); i++ {
			if routes.Paths[i].TotalEnergy < routes.Paths[index].TotalEnergy {
				index = i
			}
		}
		routeList.Lco2G = routes.Paths[index]

		// for balanced route

		// First find the two fastest paths then find the path with the smallest exposure

		// If we have only one path in the result of API call
		if len(routes.Paths) == 1 {
			routeList.Balanced = routes.Paths[0]
		} else if len(routes.Paths) == 2 {
			// if the diff between two paths is less than 5 min or 500 metres, then
			if routes.Paths[0].Time-routes.Paths[1].Time < 5*60*1000 && routes.Paths[0].Distance-routes.Paths[1].Distance < 500 {
				// return path with least exposure
				if routes.Paths[0].TotalExposure < routes.Paths[1].TotalExposure {
					routeList.Balanced = routes.Paths[0]
				} else {
					routeList.Balanced = routes.Paths[1]
				}
			} else {
				if routes.Paths[0].Time < routes.Paths[1].Time {
					routeList.Balanced = routes.Paths[0]
				} else {
					routeList.Balanced = routes.Paths[1]
				}
			}
		} else {
			// Cap slice bounds to the actual number of available paths
			topN := len(routes.Paths)
			if topN > 3 {
				topN = 3
			}

			// sorting all the routes based on exposure
			sort.Slice(routes.Paths, func(i, j int) bool {
				return routes.Paths[i].TotalExposure < routes.Paths[j].TotalExposure
			})

			// sorting top routes (up to 3) based on time
			sort.SliceStable(routes.Paths[:topN], func(i, j int) bool {
				return routes.Paths[i].Time < routes.Paths[j].Time
			})

			// sorting the top two balanced(time, exposure) routes with energy
			topTwo := topN
			if topTwo > 2 {
				topTwo = 2
			}
			sort.Slice(routes.Paths[:topTwo], func(i, j int) bool {
				return routes.Paths[i].TotalEnergy < routes.Paths[j].TotalEnergy
			})

			routeList.Balanced = routes.Paths[0]

		}

		c.IndentedJSON(http.StatusOK, routeList)

	} else if mode == "driving-traffic" {
		fmt.Println("@@@@@@@@@@@@@ Finding the route for Car @@@@@@@@@@@@@@@@")
		var mapboxRoute mapbox.RouteData = findMapboxRoute(source, destination, delayCode)

		var graphhopperRoute graphhopper.RouteData = findGraphhopperRoute(source, destination, "car")

		// Exposure and Energy calculations
		for i := 0; i < len(mapboxRoute.Routes); i++ {
			mapboxRoute.Routes[i] = utils.CalculateRouteExposureMapbox(mapboxRoute.Routes[i], delayCode)
			graphhopperRoute.Paths[i].TotalExposure = mapboxRoute.Routes[i].TotalExposure

			mapboxRoute.Routes[i].Duration *= 1000

			mapboxRoute.Routes[i].TotalEnergy = utils.CalculateRouteEnergy(graphhopperRoute.Paths[i], mode)
			graphhopperRoute.Paths[i].TotalEnergy = mapboxRoute.Routes[i].TotalEnergy

			fmt.Println("Total Exposure: ", mapboxRoute.Routes[i].TotalExposure)
			fmt.Println("Total Energy: ", mapboxRoute.Routes[i].TotalEnergy)
			fmt.Println("Distance: ", mapboxRoute.Routes[i].Distance)
			fmt.Println("Duration: ", mapboxRoute.Routes[i].Duration)
		}

		var routeList mapbox.RouteList

		routeList.Source = source[:]
		routeList.Destination = destination[:]
		routeList.DelayCode = delayCode
		routeList.Mode = mode
		routeList.RoutePref = routePref

		// for fastest route - will consider the mapbox route
		var index int
		for i := 0; i < len(mapboxRoute.Routes); i++ {
			if mapboxRoute.Routes[i].Duration < mapboxRoute.Routes[index].Duration {
				index = i
			}
		}
		routeList.Fastest = mapboxRoute.Routes[index]

		// for shortest route - also will consider the mapbox route
		index = 0
		for i := 0; i < len(mapboxRoute.Routes); i++ {
			if mapboxRoute.Routes[i].Distance < mapboxRoute.Routes[i].Distance {
				index = i
			}
		}
		routeList.Shortest = mapboxRoute.Routes[index]

		// for leap route - will consider the graphhopper route
		index = 0
		for i := 0; i < len(graphhopperRoute.Paths); i++ {
			if graphhopperRoute.Paths[i].TotalExposure < graphhopperRoute.Paths[i].TotalExposure {
				index = i
			}
		}
		routeList.LeapG = graphhopperRoute.Paths[index]

		// for lco2 route - will consider the graphhopper route
		index = 0
		for i := 0; i < len(graphhopperRoute.Paths); i++ {
			if graphhopperRoute.Paths[i].TotalEnergy < graphhopperRoute.Paths[i].TotalEnergy {
				index = i
			}
		}
		routeList.Lco2G = graphhopperRoute.Paths[index]

		// for balanced route - will consider the mapbox route
		if len(mapboxRoute.Routes) == 1 {
			routeList.Balanced = mapboxRoute.Routes[0]
		}

		// If we have two paths
		if len(mapboxRoute.Routes) == 2 {
			// If the difference between paths is less than 5 min or 500 metres, then we will consider paths will least exposure,
			// Otherwise path with least time will be the balanced path
			if mapboxRoute.Routes[0].Duration-mapboxRoute.Routes[1].Duration < 5*60*1000 && mapboxRoute.Routes[0].Distance-mapboxRoute.Routes[1].Distance < 500 {
				if mapboxRoute.Routes[0].TotalExposure < mapboxRoute.Routes[1].TotalExposure {
					routeList.Balanced = mapboxRoute.Routes[0]
				} else {
					routeList.Balanced = mapboxRoute.Routes[1]
				}
			} else {
				if mapboxRoute.Routes[0].Duration < mapboxRoute.Routes[1].Duration {
					routeList.Balanced = mapboxRoute.Routes[0]
				} else {
					routeList.Balanced = mapboxRoute.Routes[1]
				}
			}
		}

		c.IndentedJSON(http.StatusOK, routeList)
	}
}

func getBooks(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"message": "Hello, World!",
	})
}

func SetReferrerPolicy() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Referrer-Policy", "no-referrer")
		c.Next()
	}
}

func main() {
	gin.SetMode(gin.ReleaseMode)
	viper.AutomaticEnv()

	if os.Getenv("RAILWAY") != "true" {
		viper.SetConfigType("env")
		viper.SetConfigFile(".env")
		if err := viper.ReadInConfig(); err != nil {
			if !os.IsNotExist(err) {
				log.Fatalf("Error while reading config file %s", err)
			}
		}
	}

	router := gin.Default()

	// router.Use(SetReferrerPolicy())
	router.Use(cors.Default())

	router.GET("/books", getBooks)
	router.POST("/route", findRoute)
	router.POST("all-routes", findAllRoutes)

	if os.Getenv("RAILWAY") == "true" {
		router.Run()
	} else {
		router.Run()
	}
}

func checkErrNil(err error) {
	if err != nil {
		log.Fatalf("Error: %s", err)
	}
}
