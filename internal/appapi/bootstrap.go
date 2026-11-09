package appapi

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"
)

// Bootstrap replaces Simple IoT's default admin/admin before the product API
// becomes ready. Existing installations are never reset.
func Bootstrap(nc *nats.Conn, dataDir string) error {
	marker := filepath.Join(dataDir, ".easyems-initialized")
	if _, err := os.Stat(marker); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}

	password := os.Getenv("EASYEMS_ADMIN_PASSWORD")
	generated := password == ""
	if generated {
		bytes := make([]byte, 24)
		if _, err := rand.Read(bytes); err != nil {
			return err
		}
		password = base64.RawURLEncoding.EncodeToString(bytes)
	}
	if len(password) < 12 {
		return fmt.Errorf("EASYEMS_ADMIN_PASSWORD must contain at least 12 characters")
	}

	deadline := time.Now().Add(10 * time.Second)
	var admin *client.User
	for time.Now().Before(deadline) {
		root, err := client.GetRootNode(nc)
		if err == nil {
			users, err := client.GetNodesType[client.User](nc, root.ID, "all")
			if err == nil {
				for i := range users {
					if users[i].Email == "admin" {
						admin = &users[i]
						break
					}
				}
			}
		}
		if admin != nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if admin == nil {
		return fmt.Errorf("initial administrator not found")
	}

	policy, _ := json.Marshal(AccessPolicy{Admin: true, Role: "admin"})
	passwordPoint := data.NewPointString(data.PointTypePass, "", password)
	passwordPoint.Origin = "easyems-bootstrap"
	policyPoint := data.NewPointString("easyemsAccess", "", string(policy))
	policyPoint.Origin = "easyems-bootstrap"
	if err := client.SendNodePoints(nc, admin.ID, data.Points{passwordPoint, policyPoint}, true); err != nil {
		return fmt.Errorf("initialize administrator: %w", err)
	}
	accounts, err := client.UserCheck(nc, "admin", password)
	if err != nil || len(accounts) == 0 {
		return fmt.Errorf("verify initialized administrator")
	}
	if generated {
		credentials := "账号: admin\n密码: " + password + "\n首次登录后请妥善保存并删除此文件。\n"
		if err := os.WriteFile(filepath.Join(dataDir, "initial-password.txt"), []byte(credentials), 0600); err != nil {
			return fmt.Errorf("save initial credentials: %w", err)
		}
	}
	// 预置演示站点（失败不阻塞初始化，只记录）。
	if err := seedDemoSite(nc); err != nil {
		fmt.Printf("easyems: seed demo site: %v\n", err)
	}
	return os.WriteFile(marker, []byte("initialized\n"), 0600)
}
