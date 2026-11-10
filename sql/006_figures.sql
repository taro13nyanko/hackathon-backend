-- 模型の設計図を商品に保存する（2026-11、箱庭用）。Cloud SQL Studio で hackathon データベースに対して実行する。
-- ★ backend をデプロイする「前」に実行すること（先にデプロイすると、商品一覧の SELECT が figure_json 列を見つけられず 500 になる）。
-- 設計図は POST /figures が返す JSON（部品 60 個まで、サーバーで 64KB までに制限）。MEDIUMTEXT は 16MB まで入る。
ALTER TABLE item
  ADD COLUMN figure_json MEDIUMTEXT NULL AFTER figure_caption;
