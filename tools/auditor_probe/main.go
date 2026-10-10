// 运维工具：木犀审核 SDK 连通性探针。
//
// 用途：绕过小灵通后端，直接用官方 SDK 打一次审核平台，确认到底是
// 「网络/服务不可达」还是「SDK/参数有问题」。
//
// 默认只调用只读接口 GetItems（不写任何数据）。在仓库根目录执行：
//
//	go run ./tools/auditor_probe -apikey "<你的apiKey>"
//	go run ./tools/auditor_probe -apikey "<key>" -ids 637416184731402242      # 查指定 hook_id
//	go run ./tools/auditor_probe -apikey "<key>" -timeout-ms 8000
//
// 附加写测试（会真实创建一个待审条目，hook_id 用时间戳，方便你随后删）：
//
//	go run ./tools/auditor_probe -apikey "<key>" -upload
//
// 说明：Region 是审核服务地址（含 /api/v1），SDK 会自行拼 /remove/get、/remove/upload。
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	sdkerrorx "github.com/muxi-Infra/auditor-Backend/sdk/v2/api/errorx"
	"github.com/muxi-Infra/auditor-Backend/sdk/v2/api/request"
	"github.com/muxi-Infra/auditor-Backend/sdk/v2/client"
	"github.com/muxi-Infra/auditor-Backend/sdk/v2/dto"
)

func mask(s string) string {
	if len(s) <= 8 {
		return "***"
	}
	return s[:4] + "***" + s[len(s)-4:]
}

func main() {
	var (
		apikey    = flag.String("apikey", "", "木犀审核 apiKey（必填）")
		region    = flag.String("region", "https://auditor.muxixyz.com/api/v1", "审核服务地址（含 /api/v1）")
		idsStr    = flag.String("ids", "0", "要查询的 hook_id，逗号分隔；0 表示该项目全部")
		timeoutMs = flag.Int("timeout-ms", 8000, "客户端超时（毫秒，SDK 默认 5000）")
		doUpload  = flag.Bool("upload", false, "附加一次写测试：POST /remove/upload（会真实创建条目）")
	)
	flag.Parse()

	if *apikey == "" {
		fmt.Fprintln(os.Stderr, "缺少 -apikey")
		flag.Usage()
		os.Exit(2)
	}

	fmt.Printf("region   : %s\n", *region)
	fmt.Printf("apikey   : %s\n", mask(*apikey))
	fmt.Printf("timeout  : %dms\n", *timeoutMs)
	fmt.Println(strings.Repeat("-", 60))

	cli, err := client.NewClient(client.Config{
		ApiKey:         *apikey,
		Region:         *region,
		ConnectTimeout: *timeoutMs,
	})
	if err != nil {
		fmt.Printf("[构造客户端失败] %v\n", err)
		os.Exit(1)
	}

	// ---- 只读：GetItems ----
	ids := parseIntList(*idsStr)

	req, err := request.NewGetItemsStatusReq(ids)
	if err != nil {
		fmt.Printf("[构造 GetItems 请求失败] %v\n", err)
		os.Exit(1)
	}

	start := time.Now()
	resp, err := cli.GetItems(context.Background(), req)
	elapsed := time.Since(start)

	fmt.Printf("[GetItems] ids=%v 耗时=%s\n", ids, elapsed)
	if err != nil {
		fmt.Printf("  SDK 返回 error            : %v\n", err)
	} else {
		fmt.Printf("  Basic.Code                : %d\n", resp.Basic.Code)
		fmt.Printf("  Basic.Msg                 : %s\n", resp.Basic.Msg)
		if resp.Basic.Errorx != nil {
			fmt.Printf("  Basic.Errorx              : %v\n", resp.Basic.Errorx)
		}
		ok := resp.Basic.Code == sdkerrorx.SuccessCode
		fmt.Printf("  是否成功(==SuccessCode)   : %v\n", ok)
		fmt.Printf("  Items 条数                : %d\n", len(resp.Items))
		for i, it := range resp.Items {
			if i >= 10 {
				fmt.Printf("  ...（仅显示前 10 条）\n")
				break
			}
			fmt.Printf("    #%d id=%d status=%s\n", i+1, it.Id, it.Status)
		}
	}

	if !*doUpload {
		return
	}

	// ---- 写测试：UploadItem ----
	testID := uint(time.Now().UnixNano())
	hookURL := "http://example.com/auditor-probe-hook"
	ctt := dto.NewContents(dto.WithTopicText("连通性测试", "auditor-probe 写入测试，可删除"))
	upReq, err := request.NewUploadReq(hookURL, testID, ctt,
		request.WithUploadAuthor("auditor-probe"),
		request.WithUploadPublicTime(time.Now().Unix()),
		request.WithUploadTags([]string{"probe"}),
	)
	if err != nil {
		fmt.Printf("[构造 UploadReq 失败] %v\n", err)
		os.Exit(1)
	}

	fmt.Println(strings.Repeat("-", 60))
	start = time.Now()
	upResp, err := cli.UploadItem(context.Background(), upReq)
	elapsed = time.Since(start)

	fmt.Printf("[UploadItem] hook_id=%d 耗时=%s\n", testID, elapsed)
	fmt.Printf("  提示：成功后可在审核库执行 DELETE FROM items WHERE hook_id=%d AND project_id=<你的project>;\n", testID)
	if err != nil {
		fmt.Printf("  SDK 返回 error            : %v\n", err)
		return
	}
	fmt.Printf("  Basic.Code                : %d\n", upResp.Basic.Code)
	fmt.Printf("  Basic.Msg                 : %s\n", upResp.Basic.Msg)
	if upResp.Basic.Errorx != nil {
		fmt.Printf("  Basic.Errorx              : %v\n", upResp.Basic.Errorx)
	}
	fmt.Printf("  是否成功(==SuccessCode)   : %v\n", upResp.Basic.Code == sdkerrorx.SuccessCode)
	fmt.Printf("  返回 ItemID               : %d\n", upResp.ItemID)
}

func parseIntList(s string) []int {
	parts := strings.Split(s, ",")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			fmt.Printf("[-ids 解析失败] %q\n", p)
			os.Exit(2)
		}
		out = append(out, n)
	}
	if len(out) == 0 {
		out = []int{0}
	}
	return out
}
