package headscale

import (
	"database/sql"
)

// SaveKeyIP 保存 key 和 ip 的映射关系到数据库
func SaveKeyIP(db *sql.DB, key, ip string) error {
	_, err := db.Exec(
		"INSERT INTO register_key_ip_map (reg_key, ip_address) VALUES ($1, $2) ON CONFLICT (reg_key) DO UPDATE SET ip_address = EXCLUDED.ip_address",
		key, ip,
	)
	return err
}
