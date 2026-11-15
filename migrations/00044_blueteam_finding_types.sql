-- +goose Up
-- Tipe temuan blue team harus terdaftar: findings.type punya foreign key ke finding_types.
-- Tanpa ini setiap penulisan blueteam.Write dan temuan net_connection dari agent gagal dan
-- transaksi ticker dibatalkan, sehingga baseline dan deteksi tidak pernah tersimpan.
INSERT INTO finding_types (key, name, default_severity) VALUES
  ('agent_tamper','Agent tampering','critical'),
  ('agent_unresponsive','Agent unresponsive','high'),
  ('new_listening_port','New listening port','high'),
  ('new_software_installed','New software installed','high'),
  ('new_local_account','New local account','high'),
  ('c2_suspect_connection','Suspicious outbound connection','high'),
  ('net_connection','Outbound connection','info')
ON CONFLICT (key) DO NOTHING;

-- +goose Down
DELETE FROM finding_types WHERE key IN ('agent_tamper','agent_unresponsive','new_listening_port','new_software_installed','new_local_account','c2_suspect_connection','net_connection')
  AND NOT EXISTS (SELECT 1 FROM findings f WHERE f.type=finding_types.key);
