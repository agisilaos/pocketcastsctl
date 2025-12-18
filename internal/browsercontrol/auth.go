package browsercontrol

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

type TokenCandidate struct {
	SourceKey string `json:"sourceKey"`
	Token     string `json:"token"`
}

var jwtLike = regexp.MustCompile(`^[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$`)

func (c *Controller) TokenCandidates(ctx context.Context) ([]TokenCandidate, error) {
	out, err := c.runJS(ctx, jsExtractTokenCandidates())
	if err != nil {
		return nil, err
	}

	var cands []TokenCandidate
	if err := json.Unmarshal([]byte(out), &cands); err != nil {
		return nil, fmt.Errorf("unexpected JS result: %q", out)
	}
	return cands, nil
}

func scoreTokenCandidate(c TokenCandidate) int {
	score := 0
	k := strings.ToLower(c.SourceKey)
	switch {
	case strings.Contains(k, "access"):
		score += 30
	case strings.Contains(k, "auth"):
		score += 20
	case strings.Contains(k, "token"):
		score += 10
	}
	if jwtLike.MatchString(strings.TrimPrefix(c.Token, "Bearer ")) {
		score += 100
	}
	if len(strings.TrimSpace(c.Token)) >= 40 {
		score += 5
	}
	return score
}

func jsExtractTokenCandidates() string {
	// Only return values that look like tokens to avoid leaking unrelated localStorage data.
	return `(function(){
  function isJwtLike(s){
    return typeof s === 'string' && /^[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$/.test(s);
  }

  function isTokenish(s){
    if (typeof s !== 'string') return false;
    const t = s.replace(/^Bearer\s+/i,'').trim();
    if (t.length < 20) return false;
    if (t.length > 4096) return false;
    if (isJwtLike(t)) return true;
    return /^[A-Za-z0-9._=-]{20,4096}$/.test(t);
  }

  function findInObject(obj, out){
    if (!obj || typeof obj !== 'object') return;
    if (Array.isArray(obj)){
      for (const v of obj) findInObject(v, out);
      return;
    }
    for (const k of Object.keys(obj)){
      const v = obj[k];
      if (typeof v === 'string'){
        if (isTokenish(v) && (k.toLowerCase().includes('token') || k.toLowerCase().includes('auth') || k.toLowerCase().includes('session'))) {
          out.push({sourceKey: k, token: v});
        }
      } else if (v && typeof v === 'object'){
        findInObject(v, out);
      }
    }
  }

  const out = [];
  for (let i=0; i<localStorage.length; i++){
    const key = localStorage.key(i);
    const val = localStorage.getItem(key);
    if (!val) continue;
    if (isTokenish(val) && (key.toLowerCase().includes('token') || key.toLowerCase().includes('auth') || key.toLowerCase().includes('session'))) {
      out.push({sourceKey: key, token: val});
    }
    try {
      const parsed = JSON.parse(val);
      findInObject(parsed, out);
    } catch (e) {}
  }
  return JSON.stringify(out);
})()`
}
