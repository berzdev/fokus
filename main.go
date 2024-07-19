package main

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Prometheus counter
var apiActions = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "api_actions_total",
	Help: "API actions"},
	[]string{"path", "service", "status_code"},
)

var backendFailure = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "backend_failure",
	Help: "The total number of backend errors"},
	[]string{"service"},
)

var validationFailure = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "validation_failure",
	Help: "The total number of failed parameter validations"},
	[]string{"parameter_name"},
)

var authStatus = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "auth_status",
	Help: "The total number of auth requests"},
	[]string{"status"},
)

// API specific
type hetznerFirewall struct {
	token string
	id    string
}

type service struct {
	Name                  string `json:"name"`
	Art                   string `json:"art"`
	Tages_minuten_zaehler int    `json:"tages_minuten_zaehler"`
	Tages_minuten_limit   int    `json:"tages_minuten_limit"`
	State                 bool   `json:"state"`
	hetznerFirewall       hetznerFirewall
}

func NewService(Name string, Tages_minuten_zaehler int, Tages_minuten_limit int) service {
	s := service{}
	s.Name = Name
	s.Tages_minuten_zaehler = Tages_minuten_zaehler
	s.Tages_minuten_limit = Tages_minuten_limit
	return s
}

func (s service) TypeHetznerFirewall(hfw hetznerFirewall) service {
	s.Art = "hetzner-firewall"
	s.hetznerFirewall = hfw
	return s
}

func (s service) Status() bool {
	return s.State
}

func (s service) LimitErreicht() bool {
	if s.Tages_minuten_zaehler > s.Tages_minuten_limit {
		return true
	} else {
		return false
	}
}

func (s *service) LimitFill() error {
	s.Tages_minuten_zaehler = s.Tages_minuten_limit + 1
	return nil
}

func (s *service) LimitReset() error {
	s.Tages_minuten_zaehler = 0
	return nil
}

func (s *service) On() (string, int, error) {
	if s.hetznerFirewall != (hetznerFirewall{}) {
		if !(s.LimitErreicht()) {
			err := HetznerFirewall(s.hetznerFirewall, "./hetznerFirewall/enable-fw.json")
			if err != nil {
				backendFailure.WithLabelValues(s.Name).Inc()
				return "", http.StatusInternalServerError, errors.New("some errors occured with hetzner api")
			}

			s.State = true

			return "Service: " + s.Name + " is activated!", 0, nil
		} else {
			return "Service: " + s.Name + " day limit is reached!", 0, nil
		}
	} else {
		return "", http.StatusInternalServerError, nil
	}
}

func (s *service) Off() (string, int, error) {
	if s.hetznerFirewall != (hetznerFirewall{}) {
		err := HetznerFirewall(s.hetznerFirewall, "./hetznerFirewall/disable-fw.json")
		if err != nil {
			backendFailure.WithLabelValues(s.Name).Inc()
			return "", http.StatusInternalServerError, errors.New("some errors occured with hetzner api")
		}
		s.State = false
		return "Service: " + s.Name + " is deactivated!", 0, nil
	} else {
		return "", http.StatusInternalServerError, errors.New("service type is not defined")
	}
}

func (s *service) MinuteTick() (string, error) {
	if s.Status() {
		s.Tages_minuten_zaehler++
	}

	if (s.LimitErreicht()) && (s.Status()) {
		s.Off()
	}
	return "", nil
}

func HetznerFirewall(hfw hetznerFirewall, firewall_json_file string) error {
	client := &http.Client{}

	//get json file
	f, err := os.Open(firewall_json_file)
	if err != nil {
		fmt.Println("Fehler mit jsonfile")
		return err
	}

	apipath := "https://api.hetzner.cloud/v1/firewalls/" + hfw.id + "/actions/set_rules"
	req, err := http.NewRequest("POST", apipath, f)
	if err != nil {
		return err
	}

	bearer_token := "Bearer " + hfw.token
	req.Header.Set("Authorization", bearer_token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	resp.Body.Close()

	return nil
}

func GetService(name string, service_list map[string]*service) (*service, error) {
	//Input validation
	v := validator.New()
	err := v.Var(name, "required,alpha")

	//create newsrv (in case of errors)
	newsrv := service{}

	if err != nil {
		//return error if service name is invalid
		validationFailure.WithLabelValues("service").Inc()
		return &newsrv, errors.New("service name is not valid (alpha only)")
	} else {
		if s, ok := service_list[name]; ok {
			//service does exist in service_list map
			return s, nil
		} else {
			//service does not exist in map
			return &newsrv, errors.New("no valid service found")
		}
	}
}

func Auth(token string) error {
	//Input validation
	v := validator.New()
	err := v.Var(token, "required,alphanum")

	if err != nil {
		authStatus.WithLabelValues("token format invalid").Inc()
		validationFailure.WithLabelValues("token").Inc()
		//return error if service name is invalid
		return errors.New("token is not in valid format (alphanumeric only)")
	} else {
		//check if token is valid
		if token == os.Getenv("API_TOKEN") {
			authStatus.WithLabelValues("success").Inc()
			return nil
		} else {
			authStatus.WithLabelValues("token invalid").Inc()
			//service does not exist in map
			return errors.New("token is invalid")
		}
	}
}

func ApiActionCounter(path string, service string, statuscode int) {
	v := validator.New()
	err := v.Var(service, "required,alpha")

	if err == nil {
		apiActions.WithLabelValues(path, service, strconv.Itoa(statuscode)).Inc()
		return
	} else {
		return
	}
}

func main() {
	//Define service(s)
	chatx := NewService("Chatx", 0, 4).TypeHetznerFirewall(hetznerFirewall{token: os.Getenv("HETZNER_TOKEN"), id: os.Getenv("HETZNER_FW_ID")})
	//Declare map of services (for finding & structure)
	var services = make(map[string]*service)
	services["chatx"] = &chatx

	r := gin.Default()

	//Prometheus
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	r.GET("/api/v1/service/on", func(c *gin.Context) {
		//Authentication
		err := Auth(c.Query("token"))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": fmt.Sprintf("%s", err)})
			return
		}

		//Get service from parameter
		s, err := GetService(c.Query("service"), services)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("%s", err)})
		} else {
			msg, httpstatus, err := s.On()

			if err != nil {
				ApiActionCounter(c.FullPath(), c.Query("service"), httpstatus)
				c.JSON(httpstatus, gin.H{"error": fmt.Sprintf("%s", err)})
			} else {
				ApiActionCounter(c.FullPath(), c.Query("service"), http.StatusOK)
				c.JSON(http.StatusOK, gin.H{
					"message": msg,
				})
			}
		}
	})

	r.GET("/api/v1/service/off", func(c *gin.Context) {
		//Authentication
		err := Auth(c.Query("token"))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": fmt.Sprintf("%s", err)})
			return
		}

		s, err := GetService(c.Query("service"), services)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("%s", err)})
		} else {
			msg, httpstatus, err := s.Off()

			if err != nil {
				ApiActionCounter(c.FullPath(), c.Query("service"), httpstatus)
				c.JSON(httpstatus, gin.H{"error": fmt.Sprintf("%s", err)})
			} else {
				ApiActionCounter(c.FullPath(), c.Query("service"), http.StatusOK)
				c.JSON(http.StatusOK, gin.H{
					"message": msg,
				})
			}
		}
	})

	r.GET("/api/v1/service/limitfill", func(c *gin.Context) {
		//Authentication
		err := Auth(c.Query("token"))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": fmt.Sprintf("%s", err)})
			return
		}

		s, err := GetService(c.Query("service"), services)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("%s", err)})
		} else {
			err := s.LimitFill()

			if err != nil {
				ApiActionCounter(c.FullPath(), c.Query("service"), http.StatusInternalServerError)
				c.JSON(http.StatusInternalServerError, gin.H{
					"error": "Limit fill has errors.",
				})
			} else {
				ApiActionCounter(c.FullPath(), c.Query("service"), http.StatusOK)
				c.JSON(http.StatusOK, gin.H{
					"message": "Limit fill ok.",
				})
			}
		}
	})

	r.GET("/api/v1/service/status", func(c *gin.Context) {
		//Authentication
		err := Auth(c.Query("token"))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": fmt.Sprintf("%s", err)})
			return
		}

		s, err := GetService(c.Query("service"), services)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("%s", err)})
		} else {
			ApiActionCounter(c.FullPath(), c.Query("service"), http.StatusOK)
			c.IndentedJSON(http.StatusOK, s)
		}
	})

	go func() {
		done := make(chan bool)
		minuteTicker := time.NewTicker(1 * time.Minute)
		for {
			select {
			case <-done:
				return
			case <-minuteTicker.C:
				//Count service every minute (tick)
				chatx.MinuteTick()

				//Reset limit
				//https://stackoverflow.com/questions/66433556/how-to-run-a-job-on-a-specific-time-every-day
				h, m, _ := time.Now().Clock()
				if m == 0 && (h == 3) {
					chatx.LimitReset()
					fmt.Println("Limit was resetted", chatx.Tages_minuten_zaehler)
				}
			}
		}
	}()

	r.Run() // listen and serve on 0.0.0.0:8080 (for windows "localhost:8080")
}
