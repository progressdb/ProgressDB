package validation

import (
    "errors"
    "fmt"
    "strconv"
    "strings"

    "progressdb/pkg/models"
)

type Rules struct {
    Required []string
    Types    map[string]string
    MaxLen   map[string]int
    Enums    map[string][]string
    WhenThen []WhenThenRule
}

type WhenThenRule struct {
    WhenPath string
    Equals   interface{}
    ThenReq  []string
}

var rules Rules

func SetRules(r Rules) { rules = r }

func ValidateMessage(m models.Message) error {
    var errs []string
    if m.Body == nil {
        errs = append(errs, "body is required")
    }
    // Build a generic map representation for traversal
    root := map[string]interface{}{
        "id":     m.ID,
        "thread": m.Thread,
        "author": m.Author,
        "ts":     m.TS,
        "body":   m.Body,
    }

    // Required paths
    for _, p := range rules.Required {
        if !existsAt(root, p) {
            errs = append(errs, fmt.Sprintf("required path missing: %s", p))
        }
    }
    // Type checks
    for p, t := range rules.Types {
        if v, ok := valueAt(root, p); ok {
            if !typeMatches(v, t) {
                errs = append(errs, fmt.Sprintf("type mismatch at %s: expected %s", p, t))
            }
        }
    }
    // Max length
    for p, max := range rules.MaxLen {
        if v, ok := valueAt(root, p); ok {
            switch vv := v.(type) {
            case string:
                if len(vv) > max {
                    errs = append(errs, fmt.Sprintf("max length exceeded at %s: %d > %d", p, len(vv), max))
                }
            case []interface{}:
                if len(vv) > max {
                    errs = append(errs, fmt.Sprintf("max length exceeded at %s: %d > %d", p, len(vv), max))
                }
            }
        }
    }
    // Enums
    for p, vals := range rules.Enums {
        if v, ok := valueAt(root, p); ok {
            s, ok2 := v.(string)
            if ok2 {
                if !contains(vals, s) {
                    errs = append(errs, fmt.Sprintf("invalid enum at %s", p))
                }
            }
        }
    }
    // When/Then
    for _, r := range rules.WhenThen {
        if v, ok := valueAt(root, r.WhenPath); ok {
            if equalsJSONValue(v, r.Equals) {
                for _, p := range r.ThenReq {
                    if !existsAt(root, p) {
                        errs = append(errs, fmt.Sprintf("required by rule (when %s == %v): %s", r.WhenPath, r.Equals, p))
                    }
                }
            }
        }
    }

    if len(errs) > 0 {
        return errors.New(strings.Join(errs, "; "))
    }
    return nil
}

func existsAt(root interface{}, path string) bool {
    _, ok := valueAt(root, path)
    return ok
}

func valueAt(root interface{}, path string) (interface{}, bool) {
    segs := strings.Split(path, ".")
    cur := root
    for _, s := range segs {
        switch node := cur.(type) {
        case map[string]interface{}:
            v, ok := node[s]
            if !ok {
                return nil, false
            }
            cur = v
        case []interface{}:
            if s == "*" {
                if len(node) == 0 {
                    return nil, false
                }
                cur = node[0]
            } else if idx, err := strconv.Atoi(s); err == nil {
                if idx < 0 || idx >= len(node) {
                    return nil, false
                }
                cur = node[idx]
            } else {
                return nil, false
            }
        default:
            return nil, false
        }
    }
    return cur, true
}

func typeMatches(v interface{}, t string) bool {
    switch strings.ToLower(t) {
    case "string":
        _, ok := v.(string)
        return ok
    case "number":
        switch v.(type) {
        case int, int32, int64, float32, float64:
            return true
        default:
            return false
        }
    case "boolean":
        _, ok := v.(bool)
        return ok
    case "object":
        _, ok := v.(map[string]interface{})
        return ok
    case "array":
        _, ok := v.([]interface{})
        return ok
    default:
        return true
    }
}

func contains(ss []string, s string) bool {
    for _, v := range ss {
        if v == s {
            return true
        }
    }
    return false
}

func equalsJSONValue(a interface{}, b interface{}) bool {
    switch av := a.(type) {
    case string:
        if bv, ok := b.(string); ok {
            return av == bv
        }
    case float64:
        switch bv := b.(type) {
        case float64:
            return av == bv
        case int:
            return av == float64(bv)
        case int64:
            return av == float64(bv)
        }
    case bool:
        if bv, ok := b.(bool); ok {
            return av == bv
        }
    case map[string]interface{}, []interface{}:
        // Not supported in simple equality; treat as not equal
        return false
    }
    return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}

