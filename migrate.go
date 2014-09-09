package migrate

import (
	"database/sql"
	"sort"
	"time"

	"github.com/coopernurse/gorp"
)

type MigrationDirection int

const (
	Up MigrationDirection = iota
	Down
)

type Migration struct {
	Id   string
	Up   string
	Down string
}

type ById []*Migration

func (b ById) Len() int           { return len(b) }
func (b ById) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }
func (b ById) Less(i, j int) bool { return b[i].Id < b[j].Id }

type MigrationRecord struct {
	Id        string    `db:"id"`
	AppliedAt time.Time `db:"applied_at"`
}

type MigrationSource interface {
	FindMigrations() ([]*Migration, error)
}

// A hardcoded set of migrations, in-memory.
type MemoryMigrationSource struct {
	Migrations []*Migration
}

var _ MigrationSource = (*MemoryMigrationSource)(nil)

func (m MemoryMigrationSource) FindMigrations() ([]*Migration, error) {
	return m.Migrations, nil
}

// Execute a set of migrations
//
// Returns the number of applied migrations.
func Exec(db *gorp.DbMap, m MigrationSource) (int, error) {
	dbMap := &gorp.DbMap{Db: db.Db, Dialect: db.Dialect}
	dbMap.AddTableWithName(MigrationRecord{}, "gorp_migrations").SetKeys(false, "Id")
	//dbMap.TraceOn("", log.New(os.Stdout, "migrate: ", log.Lmicroseconds))

	// Make sure we have the migrations table
	err := dbMap.CreateTablesIfNotExists()
	if err != nil {
		return 0, err
	}

	migrations, err := m.FindMigrations()
	if err != nil {
		return 0, err
	}

	// Make sure migrations are sorted
	sort.Sort(ById(migrations))

	// Find the newest applied migration
	var record MigrationRecord
	err = dbMap.SelectOne(&record, "SELECT * FROM gorp_migrations ORDER BY id DESC LIMIT 1")
	if err != nil && err != sql.ErrNoRows {
		return 0, err
	}

	// Figure out which of the supplied migrations has been applied.
	toApply := ToApply(migrations, record.Id, Up)

	// Apply migrations
	applied := 0
	for _, migration := range toApply {
		_, err := dbMap.Exec(migration.Up)
		if err != nil {
			return applied, err
		}

		err = dbMap.Insert(&MigrationRecord{
			Id:        migration.Id,
			AppliedAt: time.Now(),
		})
		if err != nil {
			return applied, err
		}

		applied++
	}

	return applied, nil
}

func ToApply(migrations []*Migration, current string, direction MigrationDirection) []*Migration {
	var index = -1
	for index < len(migrations)-1 && migrations[index+1].Id <= current {
		index++
	}
	toApply := migrations[index+1:]
	return toApply
}

// TODO: Run migration + record insert in transaction.