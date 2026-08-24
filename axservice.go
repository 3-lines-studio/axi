package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type StreamEvent struct {
	Type      string `json:"type"`
	SessionID string `json:"session_id,omitempty"`
	RunID     string `json:"run_id,omitempty"`
	Sequence  int    `json:"sequence,omitempty"`
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	Text      string `json:"text,omitempty"`
	Output    string `json:"output,omitempty"`
}

type Project struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name"`
	Path string `json:"path,omitempty"`
}

type DirectoryRoot struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type Directory struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	Kind       string `json:"kind,omitempty"`
	Registered bool   `json:"registered"`
}

type DirectoryResponse struct {
	Path        string          `json:"path,omitempty"`
	Parent      string          `json:"parent,omitempty"`
	Roots       []DirectoryRoot `json:"roots,omitempty"`
	Directories []Directory     `json:"directories,omitempty"`
}

type Session struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	Title     string `json:"title"`
	UpdatedAt string `json:"updated_at"`
}

type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type SessionDetail struct {
	Session
	Messages []Message `json:"messages"`
}

type RunSummary struct {
	ID        string `json:"id"`
	SessionID string `json:"session_id"`
	Status    string `json:"status"`
}

type attachment struct {
	runID  string
	cancel context.CancelFunc
}

type AXService struct {
	app      *application.App
	client   *http.Client
	mu       sync.Mutex
	active   map[string]attachment
	baseURL  string
	username string
	password string
}

func NewAXService() *AXService {
	return &AXService{client: &http.Client{}, active: make(map[string]attachment)}
}

func (s *AXService) Connect(baseURL, username, password string) error {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return errors.New("enter a valid HTTP or HTTPS URL")
	}

	s.mu.Lock()
	for _, item := range s.active {
		item.cancel()
	}
	s.active = make(map[string]attachment)
	s.baseURL = baseURL
	s.username = username
	s.password = password
	s.mu.Unlock()
	return nil
}

func (s *AXService) Projects() ([]Project, error) {
	var items []Project
	if err := s.getJSON("/api/projects", &items); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *AXService) Directories(path string) (DirectoryResponse, error) {
	var result DirectoryResponse
	endpoint := "/api/directories"
	if path != "" {
		endpoint += "?path=" + url.QueryEscape(path)
	}
	if err := s.getJSON(endpoint, &result); err != nil {
		return result, err
	}
	return result, nil
}

func (s *AXService) AddProject(name, path string) (Project, error) {
	var result Project
	body, err := json.Marshal(Project{Name: name, Path: path})
	if err != nil {
		return result, err
	}
	if err := s.requestJSONBody(http.MethodPost, "/api/projects", body, &result); err != nil {
		return result, err
	}
	return result, nil
}

func (s *AXService) Sessions(projectID string) ([]Session, error) {
	var items []Session
	if err := s.getJSON("/api/projects/"+url.PathEscape(projectID)+"/sessions", &items); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *AXService) NewSession(projectID string) (SessionDetail, error) {
	var item SessionDetail
	if err := s.postJSON("/api/projects/"+url.PathEscape(projectID)+"/sessions", &item); err != nil {
		return item, err
	}
	return item, nil
}

func (s *AXService) OpenSession(id string) (SessionDetail, error) {
	var item SessionDetail
	if err := s.getJSON("/api/sessions/"+url.PathEscape(id), &item); err != nil {
		return item, err
	}
	return item, nil
}

func (s *AXService) DeleteSession(id string) error {
	s.mu.Lock()
	baseURL := s.baseURL
	username := s.username
	password := s.password
	s.mu.Unlock()
	req, err := http.NewRequest(http.MethodDelete, baseURL+"/api/sessions/"+url.PathEscape(id), nil)
	if err != nil {
		return err
	}
	setAuth(req, username, password)
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return responseError(resp)
	}
	return nil
}

func (s *AXService) ResumeRuns() ([]RunSummary, error) {
	var runs []RunSummary
	if err := s.getJSON("/api/runs", &runs); err != nil {
		return nil, err
	}
	for _, run := range runs {
		s.attach(run.SessionID, run.ID, 0)
	}
	return runs, nil
}

func (s *AXService) Send(session, prompt string) error {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return errors.New("enter a message")
	}

	s.mu.Lock()
	_, running := s.active[session]
	baseURL := s.baseURL
	username := s.username
	password := s.password
	s.mu.Unlock()
	if running {
		return errors.New("this chat is already running")
	}
	if baseURL == "" {
		return errors.New("connect to AX first")
	}

	runID, err := s.createRun(baseURL, username, password, session, prompt)
	if err != nil {
		return err
	}
	s.attach(session, runID, 0)
	return nil
}

func (s *AXService) Cancel(session string) error {
	s.mu.Lock()
	item, exists := s.active[session]
	baseURL := s.baseURL
	username := s.username
	password := s.password
	s.mu.Unlock()
	if !exists {
		return nil
	}

	req, err := http.NewRequest(http.MethodPost, baseURL+"/runs/"+url.PathEscape(item.runID)+"/cancel", nil)
	if err != nil {
		return err
	}
	setAuth(req, username, password)
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return responseError(resp)
	}
	return nil
}

func (s *AXService) attach(session, runID string, after int) {
	s.mu.Lock()
	if _, exists := s.active[session]; exists {
		s.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.active[session] = attachment{runID: runID, cancel: cancel}
	baseURL := s.baseURL
	username := s.username
	password := s.password
	s.mu.Unlock()
	go s.stream(ctx, baseURL, username, password, session, runID, after)
}

func (s *AXService) getJSON(path string, result any) error {
	return s.requestJSON(http.MethodGet, path, result)
}

func (s *AXService) postJSON(path string, result any) error {
	return s.requestJSON(http.MethodPost, path, result)
}

func (s *AXService) requestJSON(method, path string, result any) error {
	return s.requestJSONBody(method, path, nil, result)
}

func (s *AXService) requestJSONBody(method, path string, body []byte, result any) error {
	s.mu.Lock()
	baseURL := s.baseURL
	username := s.username
	password := s.password
	s.mu.Unlock()
	if baseURL == "" {
		return errors.New("connect to AX first")
	}
	req, err := http.NewRequest(method, baseURL+path, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	setAuth(req, username, password)
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return responseError(resp)
	}
	return json.NewDecoder(resp.Body).Decode(result)
}

func (s *AXService) createRun(baseURL, username, password, session, prompt string) (string, error) {
	body := url.Values{"prompt": {prompt}}.Encode()
	req, err := http.NewRequest(http.MethodPost, baseURL+"/sessions/"+url.PathEscape(session)+"/messages", strings.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	setAuth(req, username, password)
	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", responseError(resp)
	}
	var result struct {
		RunID string `json:"run_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || result.RunID == "" {
		return "", errors.New("AX returned an invalid run")
	}
	return result.RunID, nil
}

func (s *AXService) stream(ctx context.Context, baseURL, username, password, session, runID string, after int) {
	defer func() {
		s.mu.Lock()
		delete(s.active, session)
		s.mu.Unlock()
	}()

	endpoint := baseURL + "/runs/" + url.PathEscape(runID) + "/events?after=" + strconv.Itoa(after)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		s.emitEvent(StreamEvent{Type: "failure", SessionID: session, RunID: runID, Text: err.Error()})
		return
	}
	setAuth(req, username, password)
	resp, err := s.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		s.emitEvent(StreamEvent{Type: "failure", SessionID: session, RunID: runID, Text: err.Error()})
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		s.emitEvent(StreamEvent{Type: "failure", SessionID: session, RunID: runID, Text: responseError(resp).Error()})
		return
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	eventType := "message"
	sequence := after
	for scanner.Scan() {
		line := scanner.Text()
		if value, ok := strings.CutPrefix(line, "id: "); ok {
			sequence, _ = strconv.Atoi(value)
			continue
		}
		if value, ok := strings.CutPrefix(line, "event: "); ok {
			eventType = value
			continue
		}
		value, ok := strings.CutPrefix(line, "data: ")
		if !ok {
			continue
		}
		event := StreamEvent{Type: eventType, SessionID: session, RunID: runID, Sequence: sequence}
		if value != "null" {
			if strings.HasPrefix(value, "{") {
				if err := json.Unmarshal([]byte(value), &event); err != nil {
					continue
				}
				event.Type = eventType
				event.SessionID = session
				event.RunID = runID
				event.Sequence = sequence
			} else if err := json.Unmarshal([]byte(value), &event.Text); err != nil {
				continue
			}
		}
		s.emitEvent(event)
		if eventType == "done" || eventType == "failure" {
			return
		}
		eventType = "message"
	}
	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		s.emitEvent(StreamEvent{Type: "failure", SessionID: session, RunID: runID, Text: err.Error()})
	}
}

func (s *AXService) emitEvent(event StreamEvent) {
	s.app.Event.Emit("ax:event", event)
}

func setAuth(req *http.Request, username, password string) {
	if username != "" || password != "" {
		req.SetBasicAuth(username, password)
	}
}

func responseError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	message := strings.TrimSpace(string(body))
	if message == "" {
		message = resp.Status
	}
	return fmt.Errorf("AX: %s", message)
}
