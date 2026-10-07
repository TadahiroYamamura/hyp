package research

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

type Status string

const (
	StatusUntested  Status = "untested"
	StatusTesting   Status = "testing"
	StatusSupported Status = "supported"
	StatusRejected  Status = "rejected"
	StatusOnHold    Status = "on-hold"
)

var Statuses = []Status{StatusUntested, StatusTesting, StatusSupported, StatusRejected, StatusOnHold}

func ParseStatus(s string) (Status, error) {
	for _, v := range Statuses {
		if string(v) == s {
			return v, nil
		}
	}
	return "", fmt.Errorf("invalid status %q (want one of %s)", s, joinStrings(Statuses))
}

type Combine string

const (
	CombineAnd   Combine = "and"
	CombineOr    Combine = "or"
	CombineCases Combine = "cases"
)

var Combines = []Combine{CombineAnd, CombineOr, CombineCases}

// ParseCombine は空文字列を「combine を外す」指定として受け付ける。
func ParseCombine(s string) (Combine, error) {
	if s == "" {
		return "", nil
	}
	for _, v := range Combines {
		if string(v) == s {
			return v, nil
		}
	}
	return "", fmt.Errorf("invalid combine %q (want one of %s)", s, joinStrings(Combines))
}

type Node struct {
	ID          string         `yaml:"id" json:"id"`
	Parent      string         `yaml:"parent,omitempty" json:"parent,omitempty"`
	Title       string         `yaml:"title" json:"title"`
	Description string         `yaml:"description,omitempty" json:"description,omitempty"`
	Method      string         `yaml:"method,omitempty" json:"method,omitempty"`
	Status      Status         `yaml:"status,omitempty" json:"status,omitempty"`
	Combine     Combine        `yaml:"combine,omitempty" json:"combine,omitempty"`
	Axis        string         `yaml:"axis,omitempty" json:"axis,omitempty"`
	Weight      *float64       `yaml:"weight,omitempty" json:"weight,omitempty"`
	Condition   string         `yaml:"condition,omitempty" json:"condition,omitempty"`
	Evidence    []EvidenceLink `yaml:"evidence,omitempty" json:"evidence,omitempty"`
	CreatedAt   time.Time      `yaml:"created_at" json:"created_at"`
}

func NewID() (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// Resolve は git と同じく、一意に定まる前方一致で ID を解決する。
func Resolve(nodes []*Node, prefix string) (*Node, error) {
	return resolveID(nodes, func(n *Node) string { return n.ID }, prefix, "hypothesis")
}

func resolveID[T any](items []T, id func(T) string, prefix, kind string) (T, error) {
	var zero T
	if prefix == "" {
		return zero, fmt.Errorf("empty id")
	}
	var found []T
	for _, it := range items {
		if strings.HasPrefix(id(it), prefix) {
			found = append(found, it)
		}
	}
	switch len(found) {
	case 0:
		return zero, fmt.Errorf("no %s matches %q", kind, prefix)
	case 1:
		return found[0], nil
	default:
		ids := make([]string, len(found))
		for i, it := range found {
			ids[i] = id(it)
		}
		return zero, fmt.Errorf("ambiguous id %q: %s", prefix, strings.Join(ids, ", "))
	}
}

func joinStrings[T ~string](vs []T) string {
	ss := make([]string, len(vs))
	for i, v := range vs {
		ss[i] = string(v)
	}
	return strings.Join(ss, ", ")
}

// ParseWeight は空文字列を「重みを外す」指定として nil を返す。
func ParseWeight(s string) (*float64, error) {
	if s == "" {
		return nil, nil
	}
	w, err := strconv.ParseFloat(s, 64)
	if err != nil || !(w > 0) || math.IsInf(w, 0) {
		return nil, fmt.Errorf("invalid weight %q (want a positive number)", s)
	}
	return &w, nil
}
