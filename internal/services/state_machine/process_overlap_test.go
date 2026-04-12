package state_machine

import (
	"context"
	"database/sql"
	"reflect"
	"testing"
	"time"
	"unsafe"
	"wappiz/pkg/db"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

type testDatabase struct {
	primary *db.Replica
}

func (t *testDatabase) Primary() *db.Replica {
	return t.primary
}

func (t *testDatabase) Close() error {
	return nil
}

func newTestReplica(conn *sql.DB) *db.Replica {
	replica := &db.Replica{}
	field := reflect.ValueOf(replica).Elem().FieldByName("db")
	reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Set(reflect.ValueOf(conn))

	return replica
}

func TestHasCustomerOverlapReturnsTrueForOverlap(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer sqlDB.Close()

	svc := &service{
		db: &testDatabase{primary: newTestReplica(sqlDB)},
	}

	tenantID := uuid.New()
	customerID := uuid.New()
	startsAt := time.Date(2026, 4, 14, 9, 0, 0, 0, time.UTC)
	endsAt := startsAt.Add(30 * time.Minute)

	queryRegex := `(?s)SELECT EXISTS \(.*a\.tenant_id = \$1.*a\.customer_id = \$2.*status NOT IN \('cancelled', 'no_show'\).*a\.starts_at < \$3.*a\.ends_at > \$4.*\) AS has_overlap`
	mock.ExpectQuery(queryRegex).
		WithArgs(tenantID, customerID, endsAt, startsAt).
		WillReturnRows(sqlmock.NewRows([]string{"has_overlap"}).AddRow(true))

	hasOverlap, err := svc.hasCustomerOverlap(context.Background(), tenantID, customerID, startsAt, endsAt)
	if err != nil {
		t.Fatalf("hasCustomerOverlap returned error: %v", err)
	}
	if !hasOverlap {
		t.Fatalf("expected hasOverlap=true, got false")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations: %v", err)
	}
}

func TestHasCustomerOverlapReturnsFalseForNonOverlappingWindow(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer sqlDB.Close()

	svc := &service{
		db: &testDatabase{primary: newTestReplica(sqlDB)},
	}

	tenantID := uuid.New()
	customerID := uuid.New()
	startsAt := time.Date(2026, 4, 14, 11, 0, 0, 0, time.UTC)
	endsAt := startsAt.Add(45 * time.Minute)

	queryRegex := `(?s)SELECT EXISTS \(.*status NOT IN \('cancelled', 'no_show'\).*a\.starts_at < \$3.*a\.ends_at > \$4.*\) AS has_overlap`
	mock.ExpectQuery(queryRegex).
		WithArgs(tenantID, customerID, endsAt, startsAt).
		WillReturnRows(sqlmock.NewRows([]string{"has_overlap"}).AddRow(false))

	hasOverlap, err := svc.hasCustomerOverlap(context.Background(), tenantID, customerID, startsAt, endsAt)
	if err != nil {
		t.Fatalf("hasCustomerOverlap returned error: %v", err)
	}
	if hasOverlap {
		t.Fatalf("expected hasOverlap=false, got true")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations: %v", err)
	}
}

func TestHasCustomerOverlapDoesNotBlockCancelledOrNoShow(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer sqlDB.Close()

	svc := &service{
		db: &testDatabase{primary: newTestReplica(sqlDB)},
	}

	tenantID := uuid.New()
	customerID := uuid.New()
	startsAt := time.Date(2026, 4, 14, 9, 0, 0, 0, time.UTC)
	endsAt := startsAt.Add(30 * time.Minute)

	// Simulate only cancelled/no_show overlaps exist in DB predicate; query should return false.
	queryRegex := `(?s)SELECT EXISTS \(.*status NOT IN \('cancelled', 'no_show'\).*\) AS has_overlap`
	mock.ExpectQuery(queryRegex).
		WithArgs(tenantID, customerID, endsAt, startsAt).
		WillReturnRows(sqlmock.NewRows([]string{"has_overlap"}).AddRow(false))

	hasOverlap, err := svc.hasCustomerOverlap(context.Background(), tenantID, customerID, startsAt, endsAt)
	if err != nil {
		t.Fatalf("hasCustomerOverlap returned error: %v", err)
	}
	if hasOverlap {
		t.Fatalf("expected hasOverlap=false when only cancelled/no_show overlaps exist")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations: %v", err)
	}
}
