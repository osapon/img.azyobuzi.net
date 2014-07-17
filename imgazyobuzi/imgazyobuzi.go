package imgazyobuzi

import (
	"encoding/json"
	"fmt"
	"github.com/azyobuzin/influxdb-go"
	"github.com/codegangsta/negroni"
	"github.com/jingweno/negroni-gorelic"
	"github.com/stretchr/graceful"
	"io/ioutil"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Context struct {
	Logger                                   *log.Logger
	Port                                     int
	NewRelicLicenseKey, NewRelicAppName      string
	RedisServer, RedisAddress, RedisPassword string
	RedisDatabase                            int
	InfluxConfig                             *influxdb.ClientConfig

	influxClient *influxdb.Client
}

func NewContextFromFile(filename string) (*Context, error) {
	b, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	ctx := new(Context)
	err = json.Unmarshal(b, ctx)
	return ctx, err
}

func (self *Context) Run() {
	if self.InfluxConfig != nil {
		c, err := influxdb.NewClient(self.InfluxConfig)
		if err != nil {
			self.Logger.Panic(err)
		}
		self.influxClient = c
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", HandleRequest(self.HandleNotFound))
	mux.HandleFunc("/regex.json", HandleRequest(self.HandleRegex))
	mux.HandleFunc("/sizes.json", HandleRequest(self.HandleSizes))
	mux.HandleFunc("/redirect.json", HandleRequest(self.HandleRedirect))

	n := negroni.Classic()
	if self.NewRelicLicenseKey != "" {
		if self.NewRelicAppName == "" {
			self.NewRelicAppName = "img.azyobuzi.net v3"
		}
		n.Use(negronigorelic.New(self.NewRelicLicenseKey, self.NewRelicAppName, true))
	}
	n.UseHandler(mux)
	graceful.Run(fmt.Sprintf(":%d", self.Port), 3*time.Second, n)
}

type ErrorModel struct {
	Code      int     `json:"code"`
	Message   string  `json:"message"`
	Exception *string `json:"exception"`
}

type ErrorInfo struct {
	Status  int
	Message string
}

const (
	RequireUriParam  = 4001
	UriNotSupported  = 4002
	InvalidSizeParam = 4003
	SelectAPI        = 4041
	APINotFound      = 4042
	PictureNotFound  = 4043
	IsNotPicture     = 4044
	IsNotVideo       = 4045
	InvalidMethod    = 4051
	UnknownError     = 5000
)

var Errors = map[int]ErrorInfo{
	4000:             ErrorInfo{400, "Bad request."},
	RequireUriParam:  ErrorInfo{400, "\"uri\" parameter is required."},
	UriNotSupported:  ErrorInfo{400, "\"uri\" parameter you requested is not supported."},
	InvalidSizeParam: ErrorInfo{400, "\"size\" parameter is invalid."},
	4040:             ErrorInfo{404, "Not Found."},
	SelectAPI:        ErrorInfo{404, "Select API."},
	APINotFound:      ErrorInfo{404, "API you requested is not found."},
	PictureNotFound:  ErrorInfo{404, "The picture you requested is not found."},
	IsNotPicture:     ErrorInfo{404, "Your request is not a picture."},
	IsNotVideo:       ErrorInfo{404, "Your request is not a video."},
	4050:             ErrorInfo{405, "The method is not allowed."},
	InvalidMethod:    ErrorInfo{405, "Call with GET or HEAD method."},
	UnknownError:     ErrorInfo{500, "Raised unknown exception on server."},
}

type ResolvingErr struct {
	Code  int
	Error error
}

type Resolver interface {
	ServiceName() (string, string)
	Regex() *regexp.Regexp
	Id(groups []string) string
	Sizes(ctx *Context, groups []string) ([]ImageInfo, ResolvingErr)
}

type resolverStruct struct {
	re *regexp.Regexp
}

type Response struct {
	Status      int
	Location    string
	Body        interface{}
	HasBody     bool
	HasLocation bool
	Cacheable   bool
}

func NewResponse(status int, body interface{}, cacheable bool) Response {
	return Response{
		Status:    status,
		Body:      body,
		HasBody:   true,
		Cacheable: cacheable,
	}
}

func NewOKResponse(body interface{}, cacheable bool) Response {
	return NewResponse(http.StatusOK, body, cacheable)
}

func NewRedirectResponse(location string, cacheable bool) Response {
	return Response{
		Status:      http.StatusFound,
		Location:    location,
		HasLocation: true,
		Cacheable:   cacheable,
	}
}

func NewErrorResponse(code int, exception *string) Response {
	info := Errors[code]
	return NewResponse(info.Status, ErrorModel{code, info.Message, exception}, false)
}

func (self Response) WriteTo(w http.ResponseWriter) {
	var body []byte
	var err error
	if self.HasBody {
		body, err = json.Marshal(self.Body)
		if err != nil {
			errStr := err.Error()
			NewErrorResponse(UnknownError, &errStr).WriteTo(w)
			return
		}
	}

	header := w.Header()
	if self.HasBody {
		header.Set("Content-Type", "application/json")
		header.Set("Content-Length", strconv.Itoa(len(body)))
	}
	if self.HasLocation {
		header.Set("Location", self.Location)
	}
	if self.Cacheable {
		header.Set("Expires", time.Now().UTC().AddDate(0, 0, 10).Format(time.RFC1123))
	}
	w.WriteHeader(self.Status)

	if self.HasBody {
		w.Write(body)
	}
}

func HandleRequest(handler func(*http.Request) Response) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, req *http.Request) {
		handler(req).WriteTo(w)
	}
}

func (self *Context) HandleNotFound(req *http.Request) Response {
	code := APINotFound
	if req.URL.Path == "/" {
		code = SelectAPI
	}
	return NewErrorResponse(code, nil)
}

func (self *Context) HandleRegex(req *http.Request) Response {
	return NewOKResponse(self.GetRegex(), false)
}

func (self *Context) HandleSizes(req *http.Request) Response {
	uri := req.URL.Query().Get("uri")
	if uri == "" {
		return NewErrorResponse(RequireUriParam, nil)
	}

	res, err := self.GetSizes(uri)
	if err.Code == 0 {
		self.WriteAccessLog(req, res.ServiceId, res.Id)
		return NewOKResponse(res, true)
	}
	var msg *string
	if err.Error != nil {
		tmp := err.Error.Error()
		msg = &tmp
	}
	return NewErrorResponse(err.Code, msg)
}

func (self *Context) HandleRedirect(req *http.Request) Response {
	q := req.URL.Query()
	uri := q.Get("uri")
	if uri == "" {
		return NewErrorResponse(RequireUriParam, nil)
	}

	size := strings.ToLower(q.Get("size"))
	switch size {
	case "":
		size = "full"
	case "full", "large", "thumb", "video":
	default:
		return NewErrorResponse(InvalidSizeParam, nil)
	}

	res, err := self.Redirect(uri, size)
	if err.Code == 0 {
		self.WriteAccessLog(req, res.ServiceId, res.Id)
		return NewRedirectResponse(res.Location, true)
	}
	var msg *string
	if err.Error != nil {
		tmp := err.Error.Error()
		msg = &tmp
	}
	return NewErrorResponse(err.Code, msg)
}

func (self *Context) WriteAccessLog(req *http.Request, service, id string) {
	if self.influxClient == nil {
		return
	}

	go func() {
		err := self.influxClient.WriteSeries([]*influxdb.Series{
			&influxdb.Series{
				"log",
				[]string{"service", "id", "version", "api", "user_agent"},
				[][]interface{}{
					[]interface{}{service, id, 3, req.URL.Path, req.UserAgent()},
				},
			},
		})

		if err != nil {
			self.Logger.Printf("InfluxDB: %v\n", err)
		}
	}()
}
