CREATE TABLE IF NOT EXISTS branch_collabs (
  id SERIAL PRIMARY KEY,
  user_id INTEGER NOT NULL,
  repo_id INTEGER NOT NULL,
  branch_pattern TEXT NOT NULL,
  access_level INTEGER NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL,
  UNIQUE (user_id, repo_id, branch_pattern),
  CONSTRAINT branch_collabs_user_id_fk
  FOREIGN KEY(user_id) REFERENCES users(id)
  ON DELETE CASCADE
  ON UPDATE CASCADE,
  CONSTRAINT branch_collabs_repo_id_fk
  FOREIGN KEY(repo_id) REFERENCES repos(id)
  ON DELETE CASCADE
  ON UPDATE CASCADE
);

CREATE INDEX IF NOT EXISTS branch_collabs_repo_idx ON branch_collabs(repo_id);
CREATE INDEX IF NOT EXISTS branch_collabs_user_repo_idx ON branch_collabs(user_id, repo_id);

CREATE TABLE IF NOT EXISTS protected_branches (
  id SERIAL PRIMARY KEY,
  repo_id INTEGER NOT NULL,
  branch_pattern TEXT NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (repo_id, branch_pattern),
  CONSTRAINT protected_branches_repo_id_fk
  FOREIGN KEY(repo_id) REFERENCES repos(id)
  ON DELETE CASCADE
  ON UPDATE CASCADE
);

CREATE INDEX IF NOT EXISTS protected_branches_repo_idx ON protected_branches(repo_id);

CREATE TABLE IF NOT EXISTS pull_requests (
  id SERIAL PRIMARY KEY,
  repo_id INTEGER NOT NULL,
  number INTEGER NOT NULL,
  creator_id INTEGER NOT NULL,
  source_branch TEXT NOT NULL,
  target_branch TEXT NOT NULL,
  title TEXT NOT NULL,
  body TEXT NOT NULL DEFAULT '',
  status INTEGER NOT NULL,
  merge_commit_sha TEXT,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL,
  merged_at TIMESTAMP,
  closed_at TIMESTAMP,
  UNIQUE (repo_id, number),
  CONSTRAINT pull_requests_repo_id_fk
  FOREIGN KEY(repo_id) REFERENCES repos(id)
  ON DELETE CASCADE
  ON UPDATE CASCADE,
  CONSTRAINT pull_requests_creator_id_fk
  FOREIGN KEY(creator_id) REFERENCES users(id)
  ON DELETE CASCADE
  ON UPDATE CASCADE
);

CREATE INDEX IF NOT EXISTS pull_requests_repo_status_idx ON pull_requests(repo_id, status);
