package converter

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/raiki02/EG/api/resp"
	"github.com/raiki02/EG/internal/model"
)

// TestFeedConvertersPropagateTargetDeleted 四类 feed 响应的 TargetDeleted 都必须从 detail 透传
// （漏掉任何一类都会让该类通知回落到头像兜底）。
func TestFeedConvertersPropagateTargetDeleted(t *testing.T) {
	if out := ToGetLikeFeedResp([]model.FeedLikeDetail{{TargetDeleted: true}}); len(out) != 1 || !out[0].TargetDeleted {
		t.Fatalf("like: TargetDeleted must propagate, got %+v", out)
	}
	if out := ToGetCollectFeedResp([]model.FeedCollectDetail{{TargetDeleted: true}}); len(out) != 1 || !out[0].TargetDeleted {
		t.Fatalf("collect: TargetDeleted must propagate, got %+v", out)
	}
	if out := ToGetCommentFeedResp([]model.FeedCommentDetail{{TargetDeleted: true}}); len(out) != 1 || !out[0].TargetDeleted {
		t.Fatalf("comment: TargetDeleted must propagate, got %+v", out)
	}
	if out := ToGetAtFeedResp([]model.FeedAtDetail{{TargetDeleted: true}}); len(out) != 1 || !out[0].TargetDeleted {
		t.Fatalf("at: TargetDeleted must propagate, got %+v", out)
	}
}

// TestFeedRespJSONFieldIsTargetDeleted 锁定响应 JSON 字段名为 targetDeleted（前端契约）。
func TestFeedRespJSONFieldIsTargetDeleted(t *testing.T) {
	b, err := json.Marshal(resp.FeedLikeResp{TargetDeleted: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"targetDeleted":true`) {
		t.Fatalf("expected json field targetDeleted, got %s", b)
	}
}
