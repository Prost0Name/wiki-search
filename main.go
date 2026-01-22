package main

import (
	"container/heap"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/http2"
)

var wikiAPIs = map[string]string{
	"en": "https://en.wikipedia.org/w/api.php",
	"ru": "https://ru.wikipedia.org/w/api.php",
	"de": "https://de.wikipedia.org/w/api.php",
	"fr": "https://fr.wikipedia.org/w/api.php",
	"es": "https://es.wikipedia.org/w/api.php",
	"it": "https://it.wikipedia.org/w/api.php",
	"pt": "https://pt.wikipedia.org/w/api.php",
	"uk": "https://uk.wikipedia.org/w/api.php",
}

type WikiNode struct {
	Title    string
	Lang     string
	Priority int
	Index    int
}

func (n WikiNode) String() string { return n.Lang + ":" + n.Title }
func (n WikiNode) Key() string    { return strings.ToLower(n.Lang + ":" + n.Title) }

type PriorityQueue []*WikiNode

func (pq PriorityQueue) Len() int           { return len(pq) }
func (pq PriorityQueue) Less(i, j int) bool { return pq[i].Priority < pq[j].Priority }
func (pq PriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].Index = i
	pq[j].Index = j
}
func (pq *PriorityQueue) Push(x interface{}) {
	n := len(*pq)
	item := x.(*WikiNode)
	item.Index = n
	*pq = append(*pq, item)
}
func (pq *PriorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.Index = -1
	*pq = old[0 : n-1]
	return item
}

type LangLink struct {
	Lang  string
	Title string
}

func (l *LangLink) UnmarshalJSON(data []byte) error {
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

type WikiResponse struct {
	Query struct {
		Pages map[string]struct {
			Title     string                   `json:"title"`
			Links     []struct{ Title string } `json:"links"`
			LangLinks []LangLink               `json:"langlinks"`
		} `json:"pages"`
	} `json:"query"`
}

type Searcher struct {
	client      *http.Client
	visitedF    sync.Map
	visitedB    sync.Map
	found       atomic.Bool
	result      []WikiNode
	resultMu    sync.Mutex
	reqCount    atomic.Int64
	ctx         context.Context
	cancel      context.CancelFunc
	targetLang  string
	targetWords map[string]bool
}

func NewSearcher(targetLang, targetTitle string) *Searcher {
	tr := &http.Transport{
		MaxIdleConns:        1000,
		MaxIdleConnsPerHost: 200,
		MaxConnsPerHost:     0,
		IdleConnTimeout:     30 * time.Second,
		DisableCompression:  false,
		ForceAttemptHTTP2:   true,
	}
	http2.ConfigureTransport(tr)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

	targetWords := make(map[string]bool)
	for _, word := range strings.Fields(strings.ToLower(targetTitle)) {
		if len(word) > 2 {
			targetWords[word] = true
		}
	}

	return &Searcher{
		client:      &http.Client{Transport: tr, Timeout: 1500 * time.Millisecond},
		ctx:         ctx,
		cancel:      cancel,
		targetLang:  targetLang,
		targetWords: targetWords,
	}
}

// Быстрая эвристика (меньше = лучше)
func (s *Searcher) heuristic(title, lang string) int {
	score := 100
	titleLower := strings.ToLower(title)

	// Бонус за совпадение языка
	if lang == s.targetLang {
		score -= 20
	}

	// Бонус за общие слова с целью
	for _, word := range strings.Fields(titleLower) {
		if len(word) > 2 && s.targetWords[word] {
			score -= 30
		}
	}

	// Бонус за подстроку цели
	for word := range s.targetWords {
		if strings.Contains(titleLower, word) {
			score -= 15
		}
	}

	// Бонус за английский
	if lang == "en" {
		score -= 10
	}

	// Штраф за длинные названия
	if len(title) > 50 {
		score += 10
	}

	return score
}

func (s *Searcher) fetch(titles []string, lang, dir string) []*WikiNode {
	if s.found.Load() || len(titles) == 0 {
		return nil
	}

	apiURL := wikiAPIs[lang]
	params := url.Values{
		"action":      {"query"},
		"format":      {"json"},
		"prop":        {"links|langlinks"},
		"titles":      {strings.Join(titles, "|")},
		"pllimit":     {"max"},
		"lllimit":     {"max"},
		"plnamespace": {"0"},
		"redirects":   {"1"},
	}

	req, _ := http.NewRequestWithContext(s.ctx, "GET", apiURL+"?"+params.Encode(), nil)
	req.Header.Set("User-Agent", "WikiRacer/5.0")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	s.reqCount.Add(1)

	var data WikiResponse
	if json.NewDecoder(resp.Body).Decode(&data) != nil {
		return nil
	}

	var own, other *sync.Map
	if dir == "F" {
		own, other = &s.visitedF, &s.visitedB
	} else {
		own, other = &s.visitedB, &s.visitedF
	}

	var newNodes []*WikiNode

	for _, page := range data.Query.Pages {
		if s.found.Load() {
			return nil
		}
		parent := WikiNode{Title: page.Title, Lang: lang}

		for _, link := range page.Links {
			child := &WikiNode{
				Title:    link.Title,
				Lang:     lang,
				Priority: s.heuristic(link.Title, lang),
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
			if _, ok := wikiAPIs[ll.Lang]; !ok || ll.Title == "" {
				continue
			}
			child := &WikiNode{
				Title:    ll.Title,
				Lang:     ll.Lang,
				Priority: s.heuristic(ll.Title, ll.Lang),
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

func (s *Searcher) buildPath(meet WikiNode) []WikiNode {
	var fwd []WikiNode
	curr := meet
	for {
		fwd = append([]WikiNode{curr}, fwd...)
		val, ok := s.visitedF.Load(curr.Key())
		if !ok || val == nil {
			break
		}
		p := val.(*WikiNode)
		if p == nil {
			break
		}
		curr = *p
	}

	var bwd []WikiNode
	if val, ok := s.visitedB.Load(meet.Key()); ok && val != nil {
		curr = *val.(*WikiNode)
		for {
			bwd = append(bwd, curr)
			val, ok := s.visitedB.Load(curr.Key())
			if !ok || val == nil {
				break
			}
			p := val.(*WikiNode)
			if p == nil {
				break
			}
			curr = *p
		}
	}

	return append(fwd, bwd...)
}

func (s *Searcher) Search(start, end, lang string) []WikiNode {
	startNode := &WikiNode{Title: start, Lang: lang, Priority: 0}
	endNode := &WikiNode{Title: end, Lang: lang, Priority: 0}

	s.visitedF.Store(startNode.Key(), (*WikiNode)(nil))
	s.visitedB.Store(endNode.Key(), (*WikiNode)(nil))

	if start == end {
		return []WikiNode{*startNode}
	}

	pqF := &PriorityQueue{}
	pqB := &PriorityQueue{}
	heap.Init(pqF)
	heap.Init(pqB)

	// Первые запросы параллельно
	var wg0 sync.WaitGroup
	var initF, initB []*WikiNode
	var muF0, muB0 sync.Mutex

	wg0.Add(2)
	go func() {
		defer wg0.Done()
		nodes := s.fetch([]string{start}, lang, "F")
		muF0.Lock()
		initF = nodes
		muF0.Unlock()
	}()
	go func() {
		defer wg0.Done()
		nodes := s.fetch([]string{end}, lang, "B")
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
	const maxPerRound = 100

	for !s.found.Load() && (pqF.Len() > 0 || pqB.Len() > 0) {
		select {
		case <-s.ctx.Done():
			return s.result
		default:
		}

		var wg sync.WaitGroup
		var muF, muB sync.Mutex
		var nextF, nextB []*WikiNode

		// Forward
		byLangF := make(map[string][]string)
		count := 0
		for pqF.Len() > 0 && count < maxPerRound {
			node := heap.Pop(pqF).(*WikiNode)
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

		// Backward
		byLangB := make(map[string][]string)
		count = 0
		for pqB.Len() > 0 && count < maxPerRound {
			node := heap.Pop(pqB).(*WikiNode)
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

func main() {
	start, end, lang := "Ибраево", "Arch Linux", "ru"
	if len(os.Args) >= 3 {
		start, end = os.Args[1], os.Args[2]
	}
	if len(os.Args) >= 4 {
		lang = os.Args[3]
	}

	t0 := time.Now()
	s := NewSearcher(lang, end)
	path := s.Search(start, end, lang)

	fmt.Printf("\n⏱️ %v | 📊 %d req\n", time.Since(t0), s.reqCount.Load())

	if len(path) > 0 {
		fmt.Printf("🎯 Путь (%d):\n", len(path))
		for i, n := range path {
			fmt.Printf("  %d. %s\n", i+1, n)
		}
	} else {
		fmt.Println("❌ Не найден")
	}
}
