package database

import "testing"

func TestNormalizeMigrationURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		databaseURL string
		want        string
	}{
		{
			name:        "postgres scheme",
			databaseURL: "postgres://logos:logos@localhost:5432/logos?sslmode=disable",
			want:        "pgx5://logos:logos@localhost:5432/logos?sslmode=disable",
		},
		{
			name:        "postgresql scheme",
			databaseURL: "postgresql://logos:logos@localhost:5432/logos?sslmode=disable",
			want:        "pgx5://logos:logos@localhost:5432/logos?sslmode=disable",
		},
		{
			name:        "already normalized",
			databaseURL: "pgx5://logos:logos@localhost:5432/logos?sslmode=disable",
			want:        "pgx5://logos:logos@localhost:5432/logos?sslmode=disable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := normalizeMigrationURL(tt.databaseURL); got != tt.want {
				t.Fatalf("normalizeMigrationURL() = %q, want %q", got, tt.want)
			}
		})
	}
}
