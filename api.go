package main

import (
	"container/heap"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/swagger"
	"golang.org/x/net/http2"

	_ "wikiracer/docs" // swagger docs
)

// @title WikiRacer API
// @version 1.0
// @description API для поиска кратчайшего пути между статьями Wikipedia
// @termsOfService http://swagger.io/terms/

// @contact.name API Support
// @contact.email support@wikiracer.local

// @license.name MIT
// @license.url https://opensource.org/licenses/MIT

// @host localhost:3000
// @BasePath /api/v1

// ============== Типы данных ==============

var apiWikiAPIs = map[string]string{
	"en": "https://en.wikipedia.org/w/api.php",
	"ru": "https://ru.wikipedia.org/w/api.php",
	"de": "https://de.wikipedia.org/w/api.php",
	"fr": "https://fr.wikipedia.org/w/api.php",
	"es": "https://es.wikipedia.org/w/api.php",
	"it": "https://it.wikipedia.org/w/api.php",
	"pt": "https://pt.wikipedia.org/w/api.php",
	"uk": "https://uk.wikipedia.org/w/api.php",
}

// Глобальный HTTP клиент с прогретыми соединениями
var globalHTTPClient *http.Client

func initGlobalClient() {
	tr := &http.Transport{
		MaxIdleConns:        1000,
		MaxIdleConnsPerHost: 200,
		MaxConnsPerHost:     0,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  false,
		ForceAttemptHTTP2:   true,
	}
	http2.ConfigureTransport(tr)
	globalHTTPClient = &http.Client{Transport: tr, Timeout: 800 * time.Millisecond}
}

// SearchRequest - запрос на поиск пути
type SearchRequest struct {
	From string `json:"from" example:"Кошка" validate:"required"`
	To   string `json:"to" example:"Теория относительности" validate:"required"`
	Lang string `json:"lang,omitempty" example:"ru"`
}

// PathStep - один шаг в пути
type PathStep struct {
	Step     int    `json:"step" example:"1"`
	Title    string `json:"title" example:"Кошка"`
	Lang     string `json:"lang" example:"ru"`
	URL      string `json:"url" example:"https://ru.wikipedia.org/wiki/Кошка"`
	FullName string `json:"full_name" example:"ru:Кошка"`
}

// Transition - переход между статьями
type Transition struct {
	From        string `json:"from" example:"Кошка"`
	To          string `json:"to" example:"Квантовая механика"`
	Type        string `json:"type" example:"link"`
	Description string `json:"description" example:"Ссылка через 'кот Шрёдингера'"`
	CheckURL    string `json:"check_url" example:"https://ru.wikipedia.org/wiki/Кошка"`
}

// SearchResponse - ответ с найденным путём
type SearchResponse struct {
	Success     bool         `json:"success" example:"true"`
	From        string       `json:"from" example:"Кошка"`
	To          string       `json:"to" example:"Теория относительности"`
	PathLength  int          `json:"path_length" example:"3"`
	Path        []PathStep   `json:"path"`
	Transitions []Transition `json:"transitions"`
	Stats       SearchStats  `json:"stats"`
}

// SearchStats - статистика поиска
type SearchStats struct {
	Duration     string  `json:"duration" example:"823.45ms"`
	DurationMs   float64 `json:"duration_ms" example:"823.45"`
	RequestCount int64   `json:"request_count" example:"2"`
}

// ErrorResponse - ответ с ошибкой
type ErrorResponse struct {
	Success bool   `json:"success" example:"false"`
	Error   string `json:"error" example:"Путь не найден"`
	Code    string `json:"code" example:"PATH_NOT_FOUND"`
}

// ============== WikiRacer Logic ==============

type APIWikiNode struct {
	Title    string
	Lang     string
	Priority int
	Index    int
}

func (n APIWikiNode) String() string { return n.Lang + ":" + n.Title }
func (n APIWikiNode) Key() string    { return strings.ToLower(n.Lang + ":" + n.Title) }

type APIPriorityQueue []*APIWikiNode

func (pq APIPriorityQueue) Len() int           { return len(pq) }
func (pq APIPriorityQueue) Less(i, j int) bool { return pq[i].Priority < pq[j].Priority }
func (pq APIPriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].Index = i
	pq[j].Index = j
}
func (pq *APIPriorityQueue) Push(x interface{}) {
	n := len(*pq)
	item := x.(*APIWikiNode)
	item.Index = n
	*pq = append(*pq, item)
}
func (pq *APIPriorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.Index = -1
	*pq = old[0 : n-1]
	return item
}

type APILangLink struct {
	Lang  string
	Title string
}

func (l *APILangLink) UnmarshalJSON(data []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if lang, ok := raw["lang"].(string); ok {
		l.Lang = lang
	}
	if title, ok := raw["*"].(string); ok {
		l.Title = title
	}
	return nil
}

type APIWikiResponse struct {
	Query struct {
		Pages map[string]struct {
			Title     string                   `json:"title"`
			Links     []struct{ Title string } `json:"links"`
			LinksHere []struct{ Title string } `json:"linkshere"`
			LangLinks []APILangLink            `json:"langlinks"`
		} `json:"pages"`
	} `json:"query"`
}

type APISearcher struct {
	client      *http.Client
	visitedF    sync.Map
	visitedB    sync.Map
	found       atomic.Bool
	result      []APIWikiNode
	resultMu    sync.Mutex
	reqCount    atomic.Int64
	ctx         context.Context
	cancel      context.CancelFunc
	targetLang  string
	startLang   string
	startWords  map[string]bool
	targetWords map[string]bool
}

func NewAPISearcher(startLang, startTitle, targetLang, targetTitle string) *APISearcher {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

	startWords := make(map[string]bool)
	for _, word := range strings.Fields(strings.ToLower(startTitle)) {
		if len(word) > 2 {
			startWords[word] = true
		}
	}

	targetWords := make(map[string]bool)
	for _, word := range strings.Fields(strings.ToLower(targetTitle)) {
		if len(word) > 2 {
			targetWords[word] = true
		}
	}

	return &APISearcher{
		client:      globalHTTPClient,
		ctx:         ctx,
		cancel:      cancel,
		startLang:   startLang,
		startWords:  startWords,
		targetLang:  targetLang,
		targetWords: targetWords,
	}
}

func guessLangAPI(title string) string {
	for _, r := range title {
		if r >= 'А' && r <= 'я' || r == 'ё' || r == 'Ё' {
			return "ru"
		}
	}
	return "en"
}

func (s *APISearcher) detectLang(title string) (string, string) {
	guessed := guessLangAPI(title)
	langs := []string{guessed}
	if guessed == "ru" {
		langs = append(langs, "en")
	} else {
		langs = append(langs, "ru")
	}

	type result struct {
		lang      string
		realTitle string
		found     bool
	}

	results := make(chan result, len(langs))
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	for _, lang := range langs {
		go func(l string) {
			apiURL := apiWikiAPIs[l]
			params := url.Values{
				"action":    {"query"},
				"format":    {"json"},
				"titles":    {title},
				"redirects": {"1"},
			}

			req, err := http.NewRequestWithContext(ctx, "GET", apiURL+"?"+params.Encode(), nil)
			if err != nil {
				results <- result{l, "", false}
				return
			}
			req.Header.Set("User-Agent", "WikiRacer/5.0")

			resp, err := s.client.Do(req)
			if err != nil {
				results <- result{l, "", false}
				return
			}
			defer resp.Body.Close()

			var data struct {
				Query struct {
					Pages map[string]struct {
						Title   string `json:"title"`
						Missing bool   `json:"missing"`
					} `json:"pages"`
				} `json:"query"`
			}
			if json.NewDecoder(resp.Body).Decode(&data) != nil {
				results <- result{l, "", false}
				return
			}

			for id, page := range data.Query.Pages {
				if id != "-1" && !page.Missing {
					results <- result{l, page.Title, true}
					return
				}
			}
			results <- result{l, "", false}
		}(lang)
	}

	foundLangs := make(map[string]string)
	for i := 0; i < len(langs); i++ {
		select {
		case r := <-results:
			if r.found {
				foundLangs[r.lang] = r.realTitle
			}
		case <-ctx.Done():
			break
		}
	}

	for _, lang := range langs {
		if realTitle, ok := foundLangs[lang]; ok {
			return lang, realTitle
		}
	}
	return "", ""
}

func (s *APISearcher) heuristic(title, lang, dir string) int {
	score := 100
	titleLower := strings.ToLower(title)

	var words map[string]bool
	var targetLang string
	if dir == "F" {
		words = s.targetWords
		targetLang = s.targetLang
	} else {
		words = s.startWords
		targetLang = s.startLang
	}

	if lang == targetLang {
		score -= 25
	}

	for _, word := range strings.Fields(titleLower) {
		if len(word) > 2 && words[word] {
			score -= 40
		}
	}

	for word := range words {
		if strings.Contains(titleLower, word) {
			score -= 20
		}
	}

	if lang == "en" || lang == "ru" {
		score -= 10
	}

	if len(title) < 20 {
		score -= 5
	}

	if len(title) > 60 {
		score += 15
	}

	return score
}

func (s *APISearcher) fetch(titles []string, lang, dir string) []*APIWikiNode {
	if s.found.Load() || len(titles) == 0 {
		return nil
	}

	apiURL := apiWikiAPIs[lang]
	var params url.Values

	if dir == "F" {
		params = url.Values{
			"action":      {"query"},
			"format":      {"json"},
			"prop":        {"links|langlinks"},
			"titles":      {strings.Join(titles, "|")},
			"pllimit":     {"max"},
			"lllimit":     {"max"},
			"plnamespace": {"0"},
			"redirects":   {"1"},
		}
	} else {
		params = url.Values{
			"action":      {"query"},
			"format":      {"json"},
			"prop":        {"linkshere|langlinks"},
			"titles":      {strings.Join(titles, "|")},
			"lhlimit":     {"max"},
			"lllimit":     {"max"},
			"lhnamespace": {"0"},
			"redirects":   {"1"},
		}
	}

	req, _ := http.NewRequestWithContext(s.ctx, "GET", apiURL+"?"+params.Encode(), nil)
	req.Header.Set("User-Agent", "WikiRacer/5.0")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	s.reqCount.Add(1)

	var data APIWikiResponse
	if json.NewDecoder(resp.Body).Decode(&data) != nil {
		return nil
	}

	var own, other *sync.Map
	if dir == "F" {
		own, other = &s.visitedF, &s.visitedB
	} else {
		own, other = &s.visitedB, &s.visitedF
	}

	var newNodes []*APIWikiNode

	for _, page := range data.Query.Pages {
		if s.found.Load() {
			return nil
		}
		parent := APIWikiNode{Title: page.Title, Lang: lang}

		var links []struct{ Title string }
		if dir == "F" {
			links = page.Links
		} else {
			links = page.LinksHere
		}

		for _, link := range links {
			child := &APIWikiNode{
				Title:    link.Title,
				Lang:     lang,
				Priority: s.heuristic(link.Title, lang, dir),
			}
			key := child.Key()

			if _, exists := other.Load(key); exists {
				if s.found.CompareAndSwap(false, true) {
					own.Store(key, &parent)
					s.resultMu.Lock()
					s.result = s.buildPath(*child)
					s.resultMu.Unlock()
					s.cancel()
					return nil
				}
			}

			if _, loaded := own.LoadOrStore(key, &parent); !loaded {
				newNodes = append(newNodes, child)
			}
		}

		for _, ll := range page.LangLinks {
			if _, ok := apiWikiAPIs[ll.Lang]; !ok || ll.Title == "" {
				continue
			}
			child := &APIWikiNode{
				Title:    ll.Title,
				Lang:     ll.Lang,
				Priority: s.heuristic(ll.Title, ll.Lang, dir),
			}
			key := child.Key()

			if _, exists := other.Load(key); exists {
				if s.found.CompareAndSwap(false, true) {
					own.Store(key, &parent)
					s.resultMu.Lock()
					s.result = s.buildPath(*child)
					s.resultMu.Unlock()
					s.cancel()
					return nil
				}
			}

			if _, loaded := own.LoadOrStore(key, &parent); !loaded {
				newNodes = append(newNodes, child)
			}
		}
	}

	return newNodes
}

func (s *APISearcher) buildPath(meet APIWikiNode) []APIWikiNode {
	var fwd []APIWikiNode
	curr := meet
	for {
		fwd = append([]APIWikiNode{curr}, fwd...)
		val, ok := s.visitedF.Load(curr.Key())
		if !ok || val == nil {
			break
		}
		p := val.(*APIWikiNode)
		if p == nil {
			break
		}
		curr = *p
	}

	var bwd []APIWikiNode
	if val, ok := s.visitedB.Load(meet.Key()); ok && val != nil {
		curr = *val.(*APIWikiNode)
		for {
			bwd = append(bwd, curr)
			val, ok := s.visitedB.Load(curr.Key())
			if !ok || val == nil {
				break
			}
			p := val.(*APIWikiNode)
			if p == nil {
				break
			}
			curr = *p
		}
	}

	return append(fwd, bwd...)
}

func (s *APISearcher) Search(start, end, lang string) []APIWikiNode {
	startLang, startTitle := lang, start
	endLang, endTitle := lang, end

	var wgDetect sync.WaitGroup
	wgDetect.Add(2)

	go func() {
		defer wgDetect.Done()
		if l, t := s.detectLang(start); l != "" {
			startLang, startTitle = l, t
		}
	}()
	go func() {
		defer wgDetect.Done()
		if l, t := s.detectLang(end); l != "" {
			endLang, endTitle = l, t
		}
	}()
	wgDetect.Wait()

	s.startLang = startLang
	s.targetLang = endLang
	s.startWords = make(map[string]bool)
	for _, word := range strings.Fields(strings.ToLower(startTitle)) {
		if len(word) > 2 {
			s.startWords[word] = true
		}
	}
	s.targetWords = make(map[string]bool)
	for _, word := range strings.Fields(strings.ToLower(endTitle)) {
		if len(word) > 2 {
			s.targetWords[word] = true
		}
	}

	startNode := &APIWikiNode{Title: startTitle, Lang: startLang, Priority: 0}
	endNode := &APIWikiNode{Title: endTitle, Lang: endLang, Priority: 0}

	s.visitedF.Store(startNode.Key(), (*APIWikiNode)(nil))
	s.visitedB.Store(endNode.Key(), (*APIWikiNode)(nil))

	if startTitle == endTitle && startLang == endLang {
		return []APIWikiNode{*startNode}
	}

	pqF := &APIPriorityQueue{}
	pqB := &APIPriorityQueue{}
	heap.Init(pqF)
	heap.Init(pqB)

	var wg0 sync.WaitGroup
	var initF, initB []*APIWikiNode
	var muF0, muB0 sync.Mutex

	wg0.Add(2)
	go func() {
		defer wg0.Done()
		nodes := s.fetch([]string{startTitle}, startLang, "F")
		muF0.Lock()
		initF = nodes
		muF0.Unlock()
	}()
	go func() {
		defer wg0.Done()
		nodes := s.fetch([]string{endTitle}, endLang, "B")
		muB0.Lock()
		initB = nodes
		muB0.Unlock()
	}()
	wg0.Wait()

	if s.found.Load() {
		s.resultMu.Lock()
		defer s.resultMu.Unlock()
		return s.result
	}

	for _, n := range initF {
		heap.Push(pqF, n)
	}
	for _, n := range initB {
		heap.Push(pqB, n)
	}

	const batchSize = 50
	const maxPerRound = 250

	for !s.found.Load() && (pqF.Len() > 0 || pqB.Len() > 0) {
		select {
		case <-s.ctx.Done():
			return s.result
		default:
		}

		var wg sync.WaitGroup
		var muF, muB sync.Mutex
		var nextF, nextB []*APIWikiNode

		byLangF := make(map[string][]string)
		count := 0
		for pqF.Len() > 0 && count < maxPerRound {
			node := heap.Pop(pqF).(*APIWikiNode)
			byLangF[node.Lang] = append(byLangF[node.Lang], node.Title)
			count++
		}

		for lang, titles := range byLangF {
			for i := 0; i < len(titles); i += batchSize {
				end := i + batchSize
				if end > len(titles) {
					end = len(titles)
				}
				batch := titles[i:end]
				wg.Add(1)
				go func(t []string, l string) {
					defer wg.Done()
					nodes := s.fetch(t, l, "F")
					if len(nodes) > 0 {
						muF.Lock()
						nextF = append(nextF, nodes...)
						muF.Unlock()
					}
				}(batch, lang)
			}
		}

		byLangB := make(map[string][]string)
		count = 0
		for pqB.Len() > 0 && count < maxPerRound {
			node := heap.Pop(pqB).(*APIWikiNode)
			byLangB[node.Lang] = append(byLangB[node.Lang], node.Title)
			count++
		}

		for lang, titles := range byLangB {
			for i := 0; i < len(titles); i += batchSize {
				end := i + batchSize
				if end > len(titles) {
					end = len(titles)
				}
				batch := titles[i:end]
				wg.Add(1)
				go func(t []string, l string) {
					defer wg.Done()
					nodes := s.fetch(t, l, "B")
					if len(nodes) > 0 {
						muB.Lock()
						nextB = append(nextB, nodes...)
						muB.Unlock()
					}
				}(batch, lang)
			}
		}

		wg.Wait()

		if s.found.Load() {
			break
		}

		for _, n := range nextF {
			heap.Push(pqF, n)
		}
		for _, n := range nextB {
			heap.Push(pqB, n)
		}
	}

	s.resultMu.Lock()
	defer s.resultMu.Unlock()
	return s.result
}

// ============== API Handlers ==============

func buildWikiURL(lang, title string) string {
	return fmt.Sprintf("https://%s.wikipedia.org/wiki/%s",
		lang, strings.ReplaceAll(url.PathEscape(title), "%2F", "/"))
}

// SearchPath godoc
// @Summary Найти путь между статьями Wikipedia
// @Description Ищет кратчайший путь между двумя статьями Wikipedia используя bidirectional search
// @Tags search
// @Accept json
// @Produce json
// @Param request body SearchRequest true "Параметры поиска"
// @Success 200 {object} SearchResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /search [post]
func SearchPath(c *fiber.Ctx) error {
	var req SearchRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(ErrorResponse{
			Success: false,
			Error:   "Неверный формат запроса",
			Code:    "INVALID_REQUEST",
		})
	}

	if req.From == "" || req.To == "" {
		return c.Status(400).JSON(ErrorResponse{
			Success: false,
			Error:   "Необходимо указать 'from' и 'to'",
			Code:    "MISSING_PARAMS",
		})
	}

	if req.Lang == "" {
		req.Lang = "ru"
	}

	t0 := time.Now()
	s := NewAPISearcher(req.Lang, req.From, req.Lang, req.To)
	path := s.Search(req.From, req.To, req.Lang)
	duration := time.Since(t0)

	if len(path) == 0 {
		return c.Status(404).JSON(ErrorResponse{
			Success: false,
			Error:   "Путь не найден",
			Code:    "PATH_NOT_FOUND",
		})
	}

	// Формируем ответ
	pathSteps := make([]PathStep, len(path))
	for i, node := range path {
		pathSteps[i] = PathStep{
			Step:     i + 1,
			Title:    node.Title,
			Lang:     node.Lang,
			URL:      buildWikiURL(node.Lang, node.Title),
			FullName: node.String(),
		}
	}

	transitions := make([]Transition, 0, len(path)-1)
	for i := 0; i < len(path)-1; i++ {
		from := path[i]
		to := path[i+1]

		t := Transition{
			From:     from.Title,
			To:       to.Title,
			CheckURL: buildWikiURL(from.Lang, from.Title),
		}

		if from.Lang == to.Lang {
			t.Type = "link"
			t.Description = fmt.Sprintf("Найти '%s' в статье '%s'", to.Title, from.Title)
		} else {
			t.Type = "interwiki"
			t.Description = fmt.Sprintf("Перейти на %s версию через меню Languages", to.Lang)
		}

		transitions = append(transitions, t)
	}

	return c.JSON(SearchResponse{
		Success:     true,
		From:        req.From,
		To:          req.To,
		PathLength:  len(path),
		Path:        pathSteps,
		Transitions: transitions,
		Stats: SearchStats{
			Duration:     duration.String(),
			DurationMs:   float64(duration.Milliseconds()) + float64(duration.Microseconds()%1000)/1000,
			RequestCount: s.reqCount.Load(),
		},
	})
}

// SearchPathGet godoc
// @Summary Найти путь между статьями Wikipedia (GET)
// @Description Ищет кратчайший путь между двумя статьями Wikipedia используя bidirectional search
// @Tags search
// @Produce json
// @Param from query string true "Начальная статья" example(Кошка)
// @Param to query string true "Конечная статья" example(Теория относительности)
// @Param lang query string false "Язык по умолчанию" example(ru)
// @Success 200 {object} SearchResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /search [get]
func SearchPathGet(c *fiber.Ctx) error {
	from := c.Query("from")
	to := c.Query("to")
	lang := c.Query("lang", "ru")

	if from == "" || to == "" {
		return c.Status(400).JSON(ErrorResponse{
			Success: false,
			Error:   "Необходимо указать параметры 'from' и 'to'",
			Code:    "MISSING_PARAMS",
		})
	}

	t0 := time.Now()
	s := NewAPISearcher(lang, from, lang, to)
	path := s.Search(from, to, lang)
	duration := time.Since(t0)

	if len(path) == 0 {
		return c.Status(404).JSON(ErrorResponse{
			Success: false,
			Error:   "Путь не найден",
			Code:    "PATH_NOT_FOUND",
		})
	}

	pathSteps := make([]PathStep, len(path))
	for i, node := range path {
		pathSteps[i] = PathStep{
			Step:     i + 1,
			Title:    node.Title,
			Lang:     node.Lang,
			URL:      buildWikiURL(node.Lang, node.Title),
			FullName: node.String(),
		}
	}

	transitions := make([]Transition, 0, len(path)-1)
	for i := 0; i < len(path)-1; i++ {
		from := path[i]
		to := path[i+1]

		t := Transition{
			From:     from.Title,
			To:       to.Title,
			CheckURL: buildWikiURL(from.Lang, from.Title),
		}

		if from.Lang == to.Lang {
			t.Type = "link"
			t.Description = fmt.Sprintf("Найти '%s' в статье '%s'", to.Title, from.Title)
		} else {
			t.Type = "interwiki"
			t.Description = fmt.Sprintf("Перейти на %s версию через меню Languages", to.Lang)
		}

		transitions = append(transitions, t)
	}

	return c.JSON(SearchResponse{
		Success:     true,
		From:        from,
		To:          to,
		PathLength:  len(path),
		Path:        pathSteps,
		Transitions: transitions,
		Stats: SearchStats{
			Duration:     duration.String(),
			DurationMs:   float64(duration.Milliseconds()) + float64(duration.Microseconds()%1000)/1000,
			RequestCount: s.reqCount.Load(),
		},
	})
}

// HealthCheck godoc
// @Summary Проверка состояния API
// @Description Возвращает статус API
// @Tags health
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /health [get]
func HealthCheck(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status":  "ok",
		"service": "WikiRacer API",
		"version": "1.0.0",
	})
}

// warmupConnections прогревает HTTP/2 соединения ко всем Wikipedia API
// Это убирает 200-300мс на первый запрос (TCP + TLS + HTTP/2 handshake)
func warmupConnections() {
	var wg sync.WaitGroup
	for lang, apiURL := range apiWikiAPIs {
		wg.Add(1)
		go func(l, u string) {
			defer wg.Done()
			params := url.Values{
				"action": {"query"},
				"format": {"json"},
				"meta":   {"siteinfo"},
			}
			req, _ := http.NewRequest("GET", u+"?"+params.Encode(), nil)
			req.Header.Set("User-Agent", "WikiRacer/5.0")
			resp, err := globalHTTPClient.Do(req)
			if err == nil {
				resp.Body.Close()
				fmt.Printf("✓ %s wiki warmed up\n", l)
			}
		}(lang, apiURL)
	}
	wg.Wait()
}

func main() {
	// Инициализация глобального HTTP клиента
	initGlobalClient()

	// Прогрев соединений при старте
	fmt.Println("🔥 Прогрев соединений к Wikipedia...")
	warmupConnections()
	fmt.Println("✅ Соединения готовы!")

	app := fiber.New(fiber.Config{
		AppName: "WikiRacer API v1.0.0",
	})

	// Middleware
	app.Use(logger.New())
	app.Use(cors.New())

	// Swagger
	app.Get("/swagger/*", swagger.HandlerDefault)

	// API routes
	api := app.Group("/api/v1")
	api.Get("/health", HealthCheck)
	api.Get("/search", SearchPathGet)
	api.Post("/search", SearchPath)

	// Root redirect
	app.Get("/", func(c *fiber.Ctx) error {
		return c.Redirect("/swagger/index.html")
	})

	fmt.Println("🚀 WikiRacer API запущен на http://localhost:3000")
	fmt.Println("📚 Swagger UI: http://localhost:3000/swagger/index.html")

	app.Listen(":3000")
}
