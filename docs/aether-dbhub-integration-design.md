# Aether + DBHub Integration Design

> **Created:** January 2026
> **Status:** Design
> **Related Documents:**
> - [DBHub Operator Build Plan](./dbhub-operator-build-plan.md)
> - [DBHub Operator Implementation](./dbhub-operator-implementation.md)
> - [MCP Kubernetes Operator Pattern](./design-patterns/mcp-kubernetes-operator-pattern.md)

## Overview

This document defines how the Aether frontend and backend will integrate with the DBHub Kubernetes operator to provide declarative database connection management through a user interface.

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              User Flow                                       │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌──────────────────┐                                                        │
│  │  Aether Frontend │                                                        │
│  │  (React + Redux) │                                                        │
│  └────────┬─────────┘                                                        │
│           │ REST API                                                         │
│           ▼                                                                  │
│  ┌──────────────────┐      ┌─────────────────┐                              │
│  │  Aether Backend  │─────▶│    Neo4j        │  Database configs stored     │
│  │  (Go + Gin)      │      │   (Graph DB)    │  as nodes with relationships │
│  └────────┬─────────┘      └─────────────────┘                              │
│           │                                                                  │
│           │ Kubernetes API                                                   │
│           ▼                                                                  │
│  ┌──────────────────┐      ┌─────────────────┐                              │
│  │  Database CRD    │◀────▶│  DBHub Operator │  Watches CRDs, manages       │
│  │  (K8s Resource)  │      │  (Controller)   │  DBHub instances             │
│  └──────────────────┘      └────────┬────────┘                              │
│                                     │                                        │
│                                     ▼                                        │
│                            ┌─────────────────┐                              │
│                            │  DBHub Instance │  Auto-configured with        │
│                            │  (Pod + Service)│  discovered databases        │
│                            └────────┬────────┘                              │
│                                     │                                        │
│                                     ▼                                        │
│                            ┌─────────────────┐                              │
│                            │  User Databases │  PostgreSQL, MySQL, etc.     │
│                            └─────────────────┘                              │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Data Flow Sequence

```
1. User creates database connection in Aether UI
   └─> Frontend dispatches Redux action

2. Redux thunk calls Aether Backend API
   └─> POST /api/v1/databases

3. Aether Backend:
   a. Creates Database node in Neo4j (stores metadata)
   b. Creates Kubernetes Secret (stores credentials)
   c. Creates Database CR in Kubernetes

4. DBHub Operator (watching Database CRs):
   a. Discovers new Database CR
   b. Fetches credentials from Secret
   c. Tests database connection
   d. Updates Database CR status (Connected/Failed)
   e. Regenerates DBHubInstance config

5. Aether Backend watches Database CR status
   └─> Updates Neo4j node with connection status

6. Frontend polls for status updates
   └─> Displays connection status to user

7. User executes SQL query
   └─> Frontend → Backend → DBHub MCP → Database
```

---

## Part 1: Aether Backend Integration

### 1.1 New Models

**File: `internal/models/database.go`**

```go
package models

import "time"

// DatabaseType represents supported database types
type DatabaseType string

const (
    DatabaseTypePostgres  DatabaseType = "postgres"
    DatabaseTypeMySQL     DatabaseType = "mysql"
    DatabaseTypeMariaDB   DatabaseType = "mariadb"
    DatabaseTypeSQLServer DatabaseType = "sqlserver"
    DatabaseTypeSQLite    DatabaseType = "sqlite"
)

// DatabaseStatus represents the connection status
type DatabaseStatus string

const (
    DatabaseStatusPending   DatabaseStatus = "Pending"
    DatabaseStatusConnected DatabaseStatus = "Connected"
    DatabaseStatusFailed    DatabaseStatus = "Failed"
    DatabaseStatusDegraded  DatabaseStatus = "Degraded"
)

// Database represents a database connection configuration
type Database struct {
    // Core identity
    ID        string `json:"id"`
    Name      string `json:"name"`

    // Multi-tenancy
    TenantID  string `json:"tenant_id"`
    SpaceID   string `json:"space_id"`
    OwnerID   string `json:"owner_id"`

    // Connection details
    Type      DatabaseType `json:"type"`
    Host      string       `json:"host"`
    Port      int          `json:"port"`
    Database  string       `json:"database"`
    SSLMode   string       `json:"ssl_mode,omitempty"`

    // Kubernetes references
    SecretName      string `json:"secret_name"`       // K8s Secret with credentials
    SecretNamespace string `json:"secret_namespace"`
    CRDName         string `json:"crd_name"`          // Database CR name
    CRDNamespace    string `json:"crd_namespace"`

    // Policy
    ReadOnly          bool `json:"readonly"`
    MaxRows           int  `json:"max_rows"`
    ConnectionTimeout int  `json:"connection_timeout"`
    QueryTimeout      int  `json:"query_timeout"`

    // Status (synced from K8s)
    Status        DatabaseStatus `json:"status"`
    StatusMessage string         `json:"status_message,omitempty"`
    LastChecked   *time.Time     `json:"last_checked,omitempty"`

    // Metadata
    Labels      map[string]string `json:"labels,omitempty"`
    Description string            `json:"description,omitempty"`
    CreatedAt   time.Time         `json:"created_at"`
    UpdatedAt   time.Time         `json:"updated_at"`
}

// DatabaseCreateRequest represents the request to create a database connection
type DatabaseCreateRequest struct {
    Name        string            `json:"name" validate:"required,safe_string,min=1,max=255"`
    Type        DatabaseType      `json:"type" validate:"required,oneof=postgres mysql mariadb sqlserver sqlite"`
    Host        string            `json:"host" validate:"required,hostname|ip"`
    Port        int               `json:"port" validate:"required,min=1,max=65535"`
    Database    string            `json:"database" validate:"required,safe_string"`
    Username    string            `json:"username" validate:"required"`
    Password    string            `json:"password" validate:"required"`
    SSLMode     string            `json:"ssl_mode,omitempty" validate:"omitempty,oneof=disable require verify-ca verify-full"`
    ReadOnly    bool              `json:"readonly"`
    MaxRows     int               `json:"max_rows,omitempty" validate:"omitempty,min=1,max=100000"`
    Labels      map[string]string `json:"labels,omitempty"`
    Description string            `json:"description,omitempty" validate:"omitempty,max=1000"`
}

// DatabaseUpdateRequest represents the request to update a database connection
type DatabaseUpdateRequest struct {
    Name        *string           `json:"name,omitempty" validate:"omitempty,safe_string,min=1,max=255"`
    Host        *string           `json:"host,omitempty" validate:"omitempty,hostname|ip"`
    Port        *int              `json:"port,omitempty" validate:"omitempty,min=1,max=65535"`
    Database    *string           `json:"database,omitempty" validate:"omitempty,safe_string"`
    Username    *string           `json:"username,omitempty"`
    Password    *string           `json:"password,omitempty"`
    SSLMode     *string           `json:"ssl_mode,omitempty" validate:"omitempty,oneof=disable require verify-ca verify-full"`
    ReadOnly    *bool             `json:"readonly,omitempty"`
    MaxRows     *int              `json:"max_rows,omitempty" validate:"omitempty,min=1,max=100000"`
    Labels      map[string]string `json:"labels,omitempty"`
    Description *string           `json:"description,omitempty" validate:"omitempty,max=1000"`
}

// DatabaseResponse represents the API response for a database
type DatabaseResponse struct {
    Database
}

// DatabaseListResponse represents a paginated list of databases
type DatabaseListResponse struct {
    Databases  []Database `json:"databases"`
    Total      int        `json:"total"`
    Page       int        `json:"page"`
    PageSize   int        `json:"page_size"`
}

// QueryRequest represents a SQL query execution request
type QueryRequest struct {
    DatabaseID string `json:"database_id" validate:"required,uuid"`
    Query      string `json:"query" validate:"required,min=1,max=100000"`
    Parameters []any  `json:"parameters,omitempty"`
}

// QueryResponse represents the SQL query execution result
type QueryResponse struct {
    Columns   []string         `json:"columns"`
    Rows      []map[string]any `json:"rows"`
    RowCount  int              `json:"row_count"`
    Truncated bool             `json:"truncated"`
    Duration  int64            `json:"duration_ms"`
}

// SchemaResponse represents database schema information
type SchemaResponse struct {
    Databases []string          `json:"databases,omitempty"`
    Schemas   []string          `json:"schemas,omitempty"`
    Tables    []TableInfo       `json:"tables,omitempty"`
}

// TableInfo represents information about a database table
type TableInfo struct {
    Name       string       `json:"name"`
    Schema     string       `json:"schema,omitempty"`
    RowCount   int64        `json:"row_count,omitempty"`
    Columns    []ColumnInfo `json:"columns,omitempty"`
}

// ColumnInfo represents information about a table column
type ColumnInfo struct {
    Name       string `json:"name"`
    Type       string `json:"type"`
    Nullable   bool   `json:"nullable"`
    PrimaryKey bool   `json:"primary_key"`
    Default    string `json:"default,omitempty"`
}
```

### 1.2 Database Service

**File: `internal/services/database_service.go`**

```go
package services

import (
    "context"
    "encoding/json"
    "fmt"
    "time"

    "github.com/google/uuid"
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/client-go/kubernetes"

    "github.com/Tributary-ai-services/aether-be/internal/database"
    "github.com/Tributary-ai-services/aether-be/internal/logger"
    "github.com/Tributary-ai-services/aether-be/internal/models"
    "github.com/Tributary-ai-services/aether-be/pkg/errors"
)

const (
    databaseCRDGroup    = "dbhub.tas.io"
    databaseCRDVersion  = "v1alpha1"
    databaseCRDResource = "databases"
    defaultNamespace    = "tas-mcp-servers"
)

// DatabaseService handles database connection management
type DatabaseService struct {
    neo4j      *database.Neo4jClient
    k8sClient  kubernetes.Interface
    dbhubSvc   *DBHubService
    logger     *logger.Logger
    namespace  string
}

// NewDatabaseService creates a new DatabaseService instance
func NewDatabaseService(
    neo4j *database.Neo4jClient,
    k8sClient kubernetes.Interface,
    dbhubSvc *DBHubService,
    log *logger.Logger,
) *DatabaseService {
    return &DatabaseService{
        neo4j:     neo4j,
        k8sClient: k8sClient,
        dbhubSvc:  dbhubSvc,
        logger:    log.WithService("database_service"),
        namespace: defaultNamespace,
    }
}

// CreateDatabase creates a new database connection
func (s *DatabaseService) CreateDatabase(ctx context.Context, req models.DatabaseCreateRequest, userID, tenantID, spaceID string) (*models.Database, error) {
    log := s.logger.WithContext(ctx)

    // Generate IDs
    dbID := uuid.New().String()
    secretName := fmt.Sprintf("db-%s-creds", dbID[:8])
    crdName := fmt.Sprintf("db-%s", dbID[:8])

    log.Info("Creating database connection",
        "database_id", dbID,
        "name", req.Name,
        "type", req.Type,
        "user_id", userID,
    )

    // 1. Create Kubernetes Secret with credentials
    secret := &corev1.Secret{
        ObjectMeta: metav1.ObjectMeta{
            Name:      secretName,
            Namespace: s.namespace,
            Labels: map[string]string{
                "app.kubernetes.io/managed-by": "aether-backend",
                "dbhub.tas.io/database-id":     dbID,
                "tas.io/tenant":                tenantID,
            },
        },
        StringData: map[string]string{
            "username": req.Username,
            "password": req.Password,
        },
    }

    _, err := s.k8sClient.CoreV1().Secrets(s.namespace).Create(ctx, secret, metav1.CreateOptions{})
    if err != nil {
        return nil, errors.Internal("Failed to create credentials secret", err)
    }

    // 2. Create Database CR in Kubernetes
    err = s.createDatabaseCR(ctx, crdName, dbID, req, secretName, tenantID, spaceID)
    if err != nil {
        // Cleanup secret on failure
        s.k8sClient.CoreV1().Secrets(s.namespace).Delete(ctx, secretName, metav1.DeleteOptions{})
        return nil, err
    }

    // 3. Create Database node in Neo4j
    db := &models.Database{
        ID:                dbID,
        Name:              req.Name,
        TenantID:          tenantID,
        SpaceID:           spaceID,
        OwnerID:           userID,
        Type:              req.Type,
        Host:              req.Host,
        Port:              req.Port,
        Database:          req.Database,
        SSLMode:           req.SSLMode,
        SecretName:        secretName,
        SecretNamespace:   s.namespace,
        CRDName:           crdName,
        CRDNamespace:      s.namespace,
        ReadOnly:          req.ReadOnly,
        MaxRows:           req.MaxRows,
        ConnectionTimeout: 30, // default
        QueryTimeout:      15, // default
        Status:            models.DatabaseStatusPending,
        Labels:            req.Labels,
        Description:       req.Description,
        CreatedAt:         time.Now(),
        UpdatedAt:         time.Now(),
    }

    if db.MaxRows == 0 {
        db.MaxRows = 1000 // default
    }

    err = s.createDatabaseNode(ctx, db)
    if err != nil {
        // Cleanup K8s resources on failure
        s.deleteDatabaseCR(ctx, crdName)
        s.k8sClient.CoreV1().Secrets(s.namespace).Delete(ctx, secretName, metav1.DeleteOptions{})
        return nil, err
    }

    log.Info("Database connection created successfully",
        "database_id", dbID,
        "crd_name", crdName,
    )

    return db, nil
}

// GetDatabase retrieves a database by ID
func (s *DatabaseService) GetDatabase(ctx context.Context, id, tenantID string) (*models.Database, error) {
    query := `
        MATCH (d:Database {id: $id, tenant_id: $tenant_id})
        RETURN d
    `
    params := map[string]interface{}{
        "id":        id,
        "tenant_id": tenantID,
    }

    result, err := s.neo4j.ExecuteQueryWithLogging(ctx, query, params)
    if err != nil {
        return nil, errors.Internal("Failed to fetch database", err)
    }

    if len(result) == 0 {
        return nil, errors.NotFound("Database not found", nil)
    }

    db, err := s.nodeToDatabase(result[0])
    if err != nil {
        return nil, err
    }

    // Sync status from Kubernetes
    s.syncDatabaseStatus(ctx, db)

    return db, nil
}

// ListDatabases retrieves all databases for a tenant/space
func (s *DatabaseService) ListDatabases(ctx context.Context, tenantID, spaceID string, page, pageSize int) (*models.DatabaseListResponse, error) {
    query := `
        MATCH (d:Database {tenant_id: $tenant_id})
        WHERE $space_id IS NULL OR d.space_id = $space_id
        RETURN d
        ORDER BY d.created_at DESC
        SKIP $skip
        LIMIT $limit
    `

    countQuery := `
        MATCH (d:Database {tenant_id: $tenant_id})
        WHERE $space_id IS NULL OR d.space_id = $space_id
        RETURN count(d) as total
    `

    params := map[string]interface{}{
        "tenant_id": tenantID,
        "space_id":  spaceID,
        "skip":      (page - 1) * pageSize,
        "limit":     pageSize,
    }

    result, err := s.neo4j.ExecuteQueryWithLogging(ctx, query, params)
    if err != nil {
        return nil, errors.Internal("Failed to fetch databases", err)
    }

    countResult, err := s.neo4j.ExecuteQueryWithLogging(ctx, countQuery, params)
    if err != nil {
        return nil, errors.Internal("Failed to count databases", err)
    }

    databases := make([]models.Database, 0, len(result))
    for _, record := range result {
        db, err := s.nodeToDatabase(record)
        if err != nil {
            continue
        }
        // Sync status from Kubernetes (async in background for performance)
        go s.syncDatabaseStatus(context.Background(), db)
        databases = append(databases, *db)
    }

    total := 0
    if len(countResult) > 0 {
        if t, ok := countResult[0]["total"].(int64); ok {
            total = int(t)
        }
    }

    return &models.DatabaseListResponse{
        Databases: databases,
        Total:     total,
        Page:      page,
        PageSize:  pageSize,
    }, nil
}

// UpdateDatabase updates a database connection
func (s *DatabaseService) UpdateDatabase(ctx context.Context, id string, req models.DatabaseUpdateRequest, tenantID string) (*models.Database, error) {
    log := s.logger.WithContext(ctx)

    // Get existing database
    db, err := s.GetDatabase(ctx, id, tenantID)
    if err != nil {
        return nil, err
    }

    // Update Neo4j node
    updates := make(map[string]interface{})
    if req.Name != nil {
        updates["name"] = *req.Name
        db.Name = *req.Name
    }
    if req.Host != nil {
        updates["host"] = *req.Host
        db.Host = *req.Host
    }
    if req.Port != nil {
        updates["port"] = *req.Port
        db.Port = *req.Port
    }
    if req.Database != nil {
        updates["database"] = *req.Database
        db.Database = *req.Database
    }
    if req.SSLMode != nil {
        updates["ssl_mode"] = *req.SSLMode
        db.SSLMode = *req.SSLMode
    }
    if req.ReadOnly != nil {
        updates["readonly"] = *req.ReadOnly
        db.ReadOnly = *req.ReadOnly
    }
    if req.MaxRows != nil {
        updates["max_rows"] = *req.MaxRows
        db.MaxRows = *req.MaxRows
    }
    if req.Description != nil {
        updates["description"] = *req.Description
        db.Description = *req.Description
    }
    if req.Labels != nil {
        labelsJSON, _ := json.Marshal(req.Labels)
        updates["labels"] = string(labelsJSON)
        db.Labels = req.Labels
    }
    updates["updated_at"] = time.Now()

    // Update credentials if provided
    if req.Username != nil || req.Password != nil {
        secretData := make(map[string][]byte)
        if req.Username != nil {
            secretData["username"] = []byte(*req.Username)
        }
        if req.Password != nil {
            secretData["password"] = []byte(*req.Password)
        }

        secret, err := s.k8sClient.CoreV1().Secrets(db.SecretNamespace).Get(ctx, db.SecretName, metav1.GetOptions{})
        if err != nil {
            return nil, errors.Internal("Failed to get credentials secret", err)
        }

        for k, v := range secretData {
            secret.Data[k] = v
        }

        _, err = s.k8sClient.CoreV1().Secrets(db.SecretNamespace).Update(ctx, secret, metav1.UpdateOptions{})
        if err != nil {
            return nil, errors.Internal("Failed to update credentials", err)
        }
    }

    // Update Neo4j
    err = s.updateDatabaseNode(ctx, id, tenantID, updates)
    if err != nil {
        return nil, err
    }

    // Update Database CR to trigger operator reconciliation
    err = s.updateDatabaseCR(ctx, db)
    if err != nil {
        log.Warn("Failed to update Database CR", "error", err)
        // Non-fatal - Neo4j is source of truth
    }

    return db, nil
}

// DeleteDatabase removes a database connection
func (s *DatabaseService) DeleteDatabase(ctx context.Context, id, tenantID string) error {
    log := s.logger.WithContext(ctx)

    // Get database to find K8s resources
    db, err := s.GetDatabase(ctx, id, tenantID)
    if err != nil {
        return err
    }

    log.Info("Deleting database connection",
        "database_id", id,
        "crd_name", db.CRDName,
    )

    // 1. Delete Database CR
    err = s.deleteDatabaseCR(ctx, db.CRDName)
    if err != nil {
        log.Warn("Failed to delete Database CR", "error", err)
    }

    // 2. Delete Secret
    err = s.k8sClient.CoreV1().Secrets(db.SecretNamespace).Delete(ctx, db.SecretName, metav1.DeleteOptions{})
    if err != nil {
        log.Warn("Failed to delete credentials secret", "error", err)
    }

    // 3. Delete Neo4j node
    query := `
        MATCH (d:Database {id: $id, tenant_id: $tenant_id})
        DETACH DELETE d
    `
    params := map[string]interface{}{
        "id":        id,
        "tenant_id": tenantID,
    }

    _, err = s.neo4j.ExecuteQueryWithLogging(ctx, query, params)
    if err != nil {
        return errors.Internal("Failed to delete database", err)
    }

    log.Info("Database connection deleted successfully", "database_id", id)

    return nil
}

// TestConnection tests a database connection
func (s *DatabaseService) TestConnection(ctx context.Context, id, tenantID string) (*models.Database, error) {
    db, err := s.GetDatabase(ctx, id, tenantID)
    if err != nil {
        return nil, err
    }

    // Use DBHub to test connection
    err = s.dbhubSvc.TestConnection(ctx, db)
    if err != nil {
        db.Status = models.DatabaseStatusFailed
        db.StatusMessage = err.Error()
    } else {
        db.Status = models.DatabaseStatusConnected
        db.StatusMessage = ""
    }

    now := time.Now()
    db.LastChecked = &now

    // Update status in Neo4j
    s.updateDatabaseStatus(ctx, db)

    return db, nil
}

// ExecuteQuery executes a SQL query against a database
func (s *DatabaseService) ExecuteQuery(ctx context.Context, req models.QueryRequest, tenantID string) (*models.QueryResponse, error) {
    // Get database
    db, err := s.GetDatabase(ctx, req.DatabaseID, tenantID)
    if err != nil {
        return nil, err
    }

    // Check if readonly and query is write operation
    if db.ReadOnly && isWriteQuery(req.Query) {
        return nil, errors.Forbidden("Database is configured as read-only", nil)
    }

    // Execute via DBHub
    return s.dbhubSvc.ExecuteQuery(ctx, db, req.Query, req.Parameters)
}

// GetSchema retrieves schema information for a database
func (s *DatabaseService) GetSchema(ctx context.Context, id, tenantID string, schemaType string) (*models.SchemaResponse, error) {
    db, err := s.GetDatabase(ctx, id, tenantID)
    if err != nil {
        return nil, err
    }

    return s.dbhubSvc.GetSchema(ctx, db, schemaType)
}

// Helper methods

func (s *DatabaseService) createDatabaseNode(ctx context.Context, db *models.Database) error {
    labelsJSON, _ := json.Marshal(db.Labels)

    query := `
        CREATE (d:Database {
            id: $id,
            name: $name,
            tenant_id: $tenant_id,
            space_id: $space_id,
            owner_id: $owner_id,
            type: $type,
            host: $host,
            port: $port,
            database: $database,
            ssl_mode: $ssl_mode,
            secret_name: $secret_name,
            secret_namespace: $secret_namespace,
            crd_name: $crd_name,
            crd_namespace: $crd_namespace,
            readonly: $readonly,
            max_rows: $max_rows,
            connection_timeout: $connection_timeout,
            query_timeout: $query_timeout,
            status: $status,
            labels: $labels,
            description: $description,
            created_at: datetime(),
            updated_at: datetime()
        })
        RETURN d
    `

    params := map[string]interface{}{
        "id":                 db.ID,
        "name":               db.Name,
        "tenant_id":          db.TenantID,
        "space_id":           db.SpaceID,
        "owner_id":           db.OwnerID,
        "type":               string(db.Type),
        "host":               db.Host,
        "port":               db.Port,
        "database":           db.Database,
        "ssl_mode":           db.SSLMode,
        "secret_name":        db.SecretName,
        "secret_namespace":   db.SecretNamespace,
        "crd_name":           db.CRDName,
        "crd_namespace":      db.CRDNamespace,
        "readonly":           db.ReadOnly,
        "max_rows":           db.MaxRows,
        "connection_timeout": db.ConnectionTimeout,
        "query_timeout":      db.QueryTimeout,
        "status":             string(db.Status),
        "labels":             string(labelsJSON),
        "description":        db.Description,
    }

    _, err := s.neo4j.ExecuteQueryWithLogging(ctx, query, params)
    return err
}

func (s *DatabaseService) updateDatabaseNode(ctx context.Context, id, tenantID string, updates map[string]interface{}) error {
    setClause := ""
    params := map[string]interface{}{
        "id":        id,
        "tenant_id": tenantID,
    }

    for key, value := range updates {
        if setClause != "" {
            setClause += ", "
        }
        setClause += fmt.Sprintf("d.%s = $%s", key, key)
        params[key] = value
    }

    query := fmt.Sprintf(`
        MATCH (d:Database {id: $id, tenant_id: $tenant_id})
        SET %s
        RETURN d
    `, setClause)

    _, err := s.neo4j.ExecuteQueryWithLogging(ctx, query, params)
    return err
}

func (s *DatabaseService) updateDatabaseStatus(ctx context.Context, db *models.Database) error {
    query := `
        MATCH (d:Database {id: $id})
        SET d.status = $status,
            d.status_message = $status_message,
            d.last_checked = datetime()
        RETURN d
    `

    params := map[string]interface{}{
        "id":             db.ID,
        "status":         string(db.Status),
        "status_message": db.StatusMessage,
    }

    _, err := s.neo4j.ExecuteQueryWithLogging(ctx, query, params)
    return err
}

func (s *DatabaseService) nodeToDatabase(record map[string]interface{}) (*models.Database, error) {
    node, ok := record["d"].(map[string]interface{})
    if !ok {
        return nil, errors.Internal("Invalid database node", nil)
    }

    db := &models.Database{}

    if id, ok := node["id"].(string); ok {
        db.ID = id
    }
    if name, ok := node["name"].(string); ok {
        db.Name = name
    }
    if tenantID, ok := node["tenant_id"].(string); ok {
        db.TenantID = tenantID
    }
    if spaceID, ok := node["space_id"].(string); ok {
        db.SpaceID = spaceID
    }
    if ownerID, ok := node["owner_id"].(string); ok {
        db.OwnerID = ownerID
    }
    if dbType, ok := node["type"].(string); ok {
        db.Type = models.DatabaseType(dbType)
    }
    if host, ok := node["host"].(string); ok {
        db.Host = host
    }
    if port, ok := node["port"].(int64); ok {
        db.Port = int(port)
    }
    if database, ok := node["database"].(string); ok {
        db.Database = database
    }
    if sslMode, ok := node["ssl_mode"].(string); ok {
        db.SSLMode = sslMode
    }
    if secretName, ok := node["secret_name"].(string); ok {
        db.SecretName = secretName
    }
    if secretNamespace, ok := node["secret_namespace"].(string); ok {
        db.SecretNamespace = secretNamespace
    }
    if crdName, ok := node["crd_name"].(string); ok {
        db.CRDName = crdName
    }
    if crdNamespace, ok := node["crd_namespace"].(string); ok {
        db.CRDNamespace = crdNamespace
    }
    if readonly, ok := node["readonly"].(bool); ok {
        db.ReadOnly = readonly
    }
    if maxRows, ok := node["max_rows"].(int64); ok {
        db.MaxRows = int(maxRows)
    }
    if status, ok := node["status"].(string); ok {
        db.Status = models.DatabaseStatus(status)
    }
    if statusMsg, ok := node["status_message"].(string); ok {
        db.StatusMessage = statusMsg
    }
    if description, ok := node["description"].(string); ok {
        db.Description = description
    }
    if labelsStr, ok := node["labels"].(string); ok && labelsStr != "" {
        json.Unmarshal([]byte(labelsStr), &db.Labels)
    }

    return db, nil
}

func (s *DatabaseService) syncDatabaseStatus(ctx context.Context, db *models.Database) {
    // Get status from Kubernetes CR
    // This is async/background - don't block on it
    // Implementation depends on dynamic client or typed client for CRDs
}

func (s *DatabaseService) createDatabaseCR(ctx context.Context, name, dbID string, req models.DatabaseCreateRequest, secretName, tenantID, spaceID string) error {
    // Use dynamic client to create unstructured Database CR
    // Implementation in kubernetes_helpers.go
    return nil
}

func (s *DatabaseService) updateDatabaseCR(ctx context.Context, db *models.Database) error {
    // Update Database CR to trigger operator reconciliation
    return nil
}

func (s *DatabaseService) deleteDatabaseCR(ctx context.Context, name string) error {
    // Delete Database CR
    return nil
}

func isWriteQuery(query string) bool {
    // Simple check for write operations
    // In production, use a proper SQL parser
    keywords := []string{"INSERT", "UPDATE", "DELETE", "DROP", "CREATE", "ALTER", "TRUNCATE"}
    upperQuery := strings.ToUpper(strings.TrimSpace(query))
    for _, kw := range keywords {
        if strings.HasPrefix(upperQuery, kw) {
            return true
        }
    }
    return false
}
```

### 1.3 DBHub Service (MCP Client)

**File: `internal/services/dbhub_service.go`**

```go
package services

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "time"

    "github.com/Tributary-ai-services/aether-be/internal/config"
    "github.com/Tributary-ai-services/aether-be/internal/logger"
    "github.com/Tributary-ai-services/aether-be/internal/models"
    "github.com/Tributary-ai-services/aether-be/pkg/errors"
)

// DBHubService handles communication with DBHub MCP server
type DBHubService struct {
    baseURL string
    client  *http.Client
    logger  *logger.Logger
}

// NewDBHubService creates a new DBHub service client
func NewDBHubService(cfg *config.DBHubConfig, log *logger.Logger) *DBHubService {
    return &DBHubService{
        baseURL: cfg.BaseURL,
        client: &http.Client{
            Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second,
        },
        logger: log.WithService("dbhub_service"),
    }
}

// MCPRequest represents an MCP tool call request
type MCPRequest struct {
    Method string                 `json:"method"`
    Params map[string]interface{} `json:"params"`
}

// MCPResponse represents an MCP tool call response
type MCPResponse struct {
    Content []MCPContent `json:"content"`
    Error   *MCPError    `json:"error,omitempty"`
}

type MCPContent struct {
    Type string `json:"type"`
    Text string `json:"text,omitempty"`
}

type MCPError struct {
    Code    int    `json:"code"`
    Message string `json:"message"`
}

// ExecuteQuery executes a SQL query via DBHub
func (s *DBHubService) ExecuteQuery(ctx context.Context, db *models.Database, query string, params []any) (*models.QueryResponse, error) {
    log := s.logger.WithContext(ctx)
    start := time.Now()

    // Build MCP request for execute_sql tool
    mcpReq := MCPRequest{
        Method: "tools/call",
        Params: map[string]interface{}{
            "name": "execute_sql",
            "arguments": map[string]interface{}{
                "source": db.CRDName, // DBHub uses source ID
                "query":  query,
            },
        },
    }

    log.Debug("Executing SQL query",
        "database_id", db.ID,
        "source", db.CRDName,
    )

    resp, err := s.callMCP(ctx, mcpReq)
    if err != nil {
        return nil, err
    }

    if resp.Error != nil {
        return nil, errors.Internal(fmt.Sprintf("DBHub error: %s", resp.Error.Message), nil)
    }

    // Parse result from MCP response
    result := &models.QueryResponse{
        Duration: time.Since(start).Milliseconds(),
    }

    if len(resp.Content) > 0 && resp.Content[0].Type == "text" {
        // Parse the JSON result from text content
        var queryResult struct {
            Columns []string           `json:"columns"`
            Rows    [][]interface{}    `json:"rows"`
        }

        if err := json.Unmarshal([]byte(resp.Content[0].Text), &queryResult); err != nil {
            return nil, errors.Internal("Failed to parse query result", err)
        }

        result.Columns = queryResult.Columns
        result.Rows = make([]map[string]any, len(queryResult.Rows))

        for i, row := range queryResult.Rows {
            rowMap := make(map[string]any)
            for j, col := range queryResult.Columns {
                if j < len(row) {
                    rowMap[col] = row[j]
                }
            }
            result.Rows[i] = rowMap
        }

        result.RowCount = len(result.Rows)
        result.Truncated = result.RowCount >= db.MaxRows
    }

    return result, nil
}

// TestConnection tests database connectivity
func (s *DBHubService) TestConnection(ctx context.Context, db *models.Database) error {
    // Execute a simple query to test connection
    _, err := s.ExecuteQuery(ctx, db, "SELECT 1", nil)
    return err
}

// GetSchema retrieves schema information
func (s *DBHubService) GetSchema(ctx context.Context, db *models.Database, schemaType string) (*models.SchemaResponse, error) {
    mcpReq := MCPRequest{
        Method: "tools/call",
        Params: map[string]interface{}{
            "name": "search_objects",
            "arguments": map[string]interface{}{
                "source": db.CRDName,
                "type":   schemaType, // "database", "schema", "table"
            },
        },
    }

    resp, err := s.callMCP(ctx, mcpReq)
    if err != nil {
        return nil, err
    }

    if resp.Error != nil {
        return nil, errors.Internal(fmt.Sprintf("DBHub error: %s", resp.Error.Message), nil)
    }

    result := &models.SchemaResponse{}

    if len(resp.Content) > 0 && resp.Content[0].Type == "text" {
        if err := json.Unmarshal([]byte(resp.Content[0].Text), result); err != nil {
            return nil, errors.Internal("Failed to parse schema result", err)
        }
    }

    return result, nil
}

func (s *DBHubService) callMCP(ctx context.Context, req MCPRequest) (*MCPResponse, error) {
    body, err := json.Marshal(req)
    if err != nil {
        return nil, errors.Internal("Failed to marshal MCP request", err)
    }

    httpReq, err := http.NewRequestWithContext(ctx, "POST", s.baseURL+"/mcp", bytes.NewReader(body))
    if err != nil {
        return nil, errors.Internal("Failed to create HTTP request", err)
    }

    httpReq.Header.Set("Content-Type", "application/json")
    httpReq.Header.Set("Accept", "application/json, text/event-stream")

    httpResp, err := s.client.Do(httpReq)
    if err != nil {
        return nil, errors.ServiceUnavailable("DBHub service unavailable", err)
    }
    defer httpResp.Body.Close()

    respBody, err := io.ReadAll(httpResp.Body)
    if err != nil {
        return nil, errors.Internal("Failed to read response", err)
    }

    if httpResp.StatusCode != http.StatusOK {
        return nil, errors.Internal(fmt.Sprintf("DBHub returned status %d: %s", httpResp.StatusCode, string(respBody)), nil)
    }

    var resp MCPResponse
    if err := json.Unmarshal(respBody, &resp); err != nil {
        return nil, errors.Internal("Failed to parse MCP response", err)
    }

    return &resp, nil
}
```

### 1.4 Database Handler

**File: `internal/handlers/database_handler.go`**

```go
package handlers

import (
    "net/http"
    "strconv"

    "github.com/gin-gonic/gin"

    "github.com/Tributary-ai-services/aether-be/internal/logger"
    "github.com/Tributary-ai-services/aether-be/internal/models"
    "github.com/Tributary-ai-services/aether-be/internal/services"
    "github.com/Tributary-ai-services/aether-be/pkg/errors"
)

// DatabaseHandler handles database connection management endpoints
type DatabaseHandler struct {
    dbService *services.DatabaseService
    logger    *logger.Logger
}

// NewDatabaseHandler creates a new DatabaseHandler
func NewDatabaseHandler(dbService *services.DatabaseService, log *logger.Logger) *DatabaseHandler {
    return &DatabaseHandler{
        dbService: dbService,
        logger:    log.WithService("database_handler"),
    }
}

// CreateDatabase creates a new database connection
// @Summary Create database connection
// @Description Create a new database connection configuration
// @Tags databases
// @Accept json
// @Produce json
// @Param body body models.DatabaseCreateRequest true "Database configuration"
// @Success 201 {object} models.DatabaseResponse
// @Failure 400 {object} errors.APIError
// @Failure 401 {object} errors.APIError
// @Failure 500 {object} errors.APIError
// @Router /api/v1/databases [post]
func (h *DatabaseHandler) CreateDatabase(c *gin.Context) {
    var req models.DatabaseCreateRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, errors.Validation("Invalid request body", err))
        return
    }

    userID, _ := c.Get("internal_user_id")
    tenantID, _ := c.Get("tenant_id")
    spaceID, _ := c.Get("space_id")

    db, err := h.dbService.CreateDatabase(c.Request.Context(), req, userID.(string), tenantID.(string), spaceID.(string))
    if err != nil {
        handleError(c, err)
        return
    }

    c.JSON(http.StatusCreated, models.DatabaseResponse{Database: *db})
}

// GetDatabase retrieves a database by ID
// @Summary Get database
// @Description Get a database connection by ID
// @Tags databases
// @Produce json
// @Param id path string true "Database ID"
// @Success 200 {object} models.DatabaseResponse
// @Failure 404 {object} errors.APIError
// @Router /api/v1/databases/{id} [get]
func (h *DatabaseHandler) GetDatabase(c *gin.Context) {
    id := c.Param("id")
    tenantID, _ := c.Get("tenant_id")

    db, err := h.dbService.GetDatabase(c.Request.Context(), id, tenantID.(string))
    if err != nil {
        handleError(c, err)
        return
    }

    c.JSON(http.StatusOK, models.DatabaseResponse{Database: *db})
}

// ListDatabases lists all databases for the tenant
// @Summary List databases
// @Description List all database connections for the current tenant/space
// @Tags databases
// @Produce json
// @Param page query int false "Page number" default(1)
// @Param page_size query int false "Page size" default(20)
// @Success 200 {object} models.DatabaseListResponse
// @Router /api/v1/databases [get]
func (h *DatabaseHandler) ListDatabases(c *gin.Context) {
    tenantID, _ := c.Get("tenant_id")
    spaceID, _ := c.Get("space_id")

    page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
    pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

    if page < 1 {
        page = 1
    }
    if pageSize < 1 || pageSize > 100 {
        pageSize = 20
    }

    spaceIDStr := ""
    if spaceID != nil {
        spaceIDStr = spaceID.(string)
    }

    result, err := h.dbService.ListDatabases(c.Request.Context(), tenantID.(string), spaceIDStr, page, pageSize)
    if err != nil {
        handleError(c, err)
        return
    }

    c.JSON(http.StatusOK, result)
}

// UpdateDatabase updates a database connection
// @Summary Update database
// @Description Update a database connection configuration
// @Tags databases
// @Accept json
// @Produce json
// @Param id path string true "Database ID"
// @Param body body models.DatabaseUpdateRequest true "Database updates"
// @Success 200 {object} models.DatabaseResponse
// @Router /api/v1/databases/{id} [put]
func (h *DatabaseHandler) UpdateDatabase(c *gin.Context) {
    id := c.Param("id")
    tenantID, _ := c.Get("tenant_id")

    var req models.DatabaseUpdateRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, errors.Validation("Invalid request body", err))
        return
    }

    db, err := h.dbService.UpdateDatabase(c.Request.Context(), id, req, tenantID.(string))
    if err != nil {
        handleError(c, err)
        return
    }

    c.JSON(http.StatusOK, models.DatabaseResponse{Database: *db})
}

// DeleteDatabase removes a database connection
// @Summary Delete database
// @Description Delete a database connection configuration
// @Tags databases
// @Param id path string true "Database ID"
// @Success 204
// @Router /api/v1/databases/{id} [delete]
func (h *DatabaseHandler) DeleteDatabase(c *gin.Context) {
    id := c.Param("id")
    tenantID, _ := c.Get("tenant_id")

    err := h.dbService.DeleteDatabase(c.Request.Context(), id, tenantID.(string))
    if err != nil {
        handleError(c, err)
        return
    }

    c.Status(http.StatusNoContent)
}

// TestConnection tests a database connection
// @Summary Test database connection
// @Description Test connectivity to a database
// @Tags databases
// @Produce json
// @Param id path string true "Database ID"
// @Success 200 {object} models.DatabaseResponse
// @Router /api/v1/databases/{id}/test [post]
func (h *DatabaseHandler) TestConnection(c *gin.Context) {
    id := c.Param("id")
    tenantID, _ := c.Get("tenant_id")

    db, err := h.dbService.TestConnection(c.Request.Context(), id, tenantID.(string))
    if err != nil {
        handleError(c, err)
        return
    }

    c.JSON(http.StatusOK, models.DatabaseResponse{Database: *db})
}

// ExecuteQuery executes a SQL query
// @Summary Execute SQL query
// @Description Execute a SQL query against a database
// @Tags databases
// @Accept json
// @Produce json
// @Param body body models.QueryRequest true "Query request"
// @Success 200 {object} models.QueryResponse
// @Router /api/v1/databases/query [post]
func (h *DatabaseHandler) ExecuteQuery(c *gin.Context) {
    var req models.QueryRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, errors.Validation("Invalid request body", err))
        return
    }

    tenantID, _ := c.Get("tenant_id")

    result, err := h.dbService.ExecuteQuery(c.Request.Context(), req, tenantID.(string))
    if err != nil {
        handleError(c, err)
        return
    }

    c.JSON(http.StatusOK, result)
}

// GetSchema retrieves database schema information
// @Summary Get database schema
// @Description Get schema information (databases, schemas, tables) for a database
// @Tags databases
// @Produce json
// @Param id path string true "Database ID"
// @Param type query string false "Schema type: database, schema, table" default(table)
// @Success 200 {object} models.SchemaResponse
// @Router /api/v1/databases/{id}/schema [get]
func (h *DatabaseHandler) GetSchema(c *gin.Context) {
    id := c.Param("id")
    schemaType := c.DefaultQuery("type", "table")
    tenantID, _ := c.Get("tenant_id")

    result, err := h.dbService.GetSchema(c.Request.Context(), id, tenantID.(string), schemaType)
    if err != nil {
        handleError(c, err)
        return
    }

    c.JSON(http.StatusOK, result)
}
```

### 1.5 Route Registration

**Add to `internal/handlers/routes.go`:**

```go
// In setupRoutes function, add after other route groups:

// Database management routes
databases := api.Group("/databases")
databases.Use(middleware.RequireSpaceContext())
{
    databases.POST("", databaseHandler.CreateDatabase)
    databases.GET("", databaseHandler.ListDatabases)
    databases.GET("/:id", databaseHandler.GetDatabase)
    databases.PUT("/:id", databaseHandler.UpdateDatabase)
    databases.DELETE("/:id", databaseHandler.DeleteDatabase)
    databases.POST("/:id/test", databaseHandler.TestConnection)
    databases.GET("/:id/schema", databaseHandler.GetSchema)
    databases.POST("/query", databaseHandler.ExecuteQuery)
}
```

### 1.6 Configuration

**Add to `internal/config/config.go`:**

```go
type DBHubConfig struct {
    BaseURL        string `env:"DBHUB_URL" envDefault:"http://dbhub.tas-mcp-servers.svc.cluster.local:8080"`
    Enabled        bool   `env:"DBHUB_ENABLED" envDefault:"true"`
    TimeoutSeconds int    `env:"DBHUB_TIMEOUT" envDefault:"30"`
}

// Add to main Config struct
type Config struct {
    // ... existing fields ...
    DBHub DBHubConfig
}
```

---

## Part 2: Aether Frontend Integration

### 2.1 TypeScript Types

**File: `src/types/database.ts`**

```typescript
// Database types
export type DatabaseType = 'postgres' | 'mysql' | 'mariadb' | 'sqlserver' | 'sqlite';
export type DatabaseStatus = 'Pending' | 'Connected' | 'Failed' | 'Degraded';

export interface Database {
  id: string;
  name: string;
  tenant_id: string;
  space_id: string;
  owner_id: string;
  type: DatabaseType;
  host: string;
  port: number;
  database: string;
  ssl_mode?: string;
  readonly: boolean;
  max_rows: number;
  connection_timeout: number;
  query_timeout: number;
  status: DatabaseStatus;
  status_message?: string;
  last_checked?: string;
  labels?: Record<string, string>;
  description?: string;
  created_at: string;
  updated_at: string;
}

export interface DatabaseCreateRequest {
  name: string;
  type: DatabaseType;
  host: string;
  port: number;
  database: string;
  username: string;
  password: string;
  ssl_mode?: string;
  readonly?: boolean;
  max_rows?: number;
  labels?: Record<string, string>;
  description?: string;
}

export interface DatabaseUpdateRequest {
  name?: string;
  host?: string;
  port?: number;
  database?: string;
  username?: string;
  password?: string;
  ssl_mode?: string;
  readonly?: boolean;
  max_rows?: number;
  labels?: Record<string, string>;
  description?: string;
}

export interface QueryRequest {
  database_id: string;
  query: string;
  parameters?: unknown[];
}

export interface QueryResponse {
  columns: string[];
  rows: Record<string, unknown>[];
  row_count: number;
  truncated: boolean;
  duration_ms: number;
}

export interface TableInfo {
  name: string;
  schema?: string;
  row_count?: number;
  columns?: ColumnInfo[];
}

export interface ColumnInfo {
  name: string;
  type: string;
  nullable: boolean;
  primary_key: boolean;
  default?: string;
}

export interface SchemaResponse {
  databases?: string[];
  schemas?: string[];
  tables?: TableInfo[];
}

export interface DatabaseListResponse {
  databases: Database[];
  total: number;
  page: number;
  page_size: number;
}
```

### 2.2 Redux Slice

**File: `src/store/slices/databaseSlice.ts`**

```typescript
import { createSlice, createAsyncThunk, PayloadAction } from '@reduxjs/toolkit';
import { databaseApi } from '../../services/api/databaseApi';
import type {
  Database,
  DatabaseCreateRequest,
  DatabaseUpdateRequest,
  QueryRequest,
  QueryResponse,
  SchemaResponse,
  DatabaseListResponse
} from '../../types/database';

interface DatabaseState {
  // Connection management
  databases: Database[];
  selectedDatabaseId: string | null;
  total: number;
  page: number;
  pageSize: number;

  // Query execution
  queryResult: QueryResponse | null;
  queryHistory: Array<{
    id: string;
    databaseId: string;
    query: string;
    executedAt: string;
    duration: number;
    rowCount: number;
  }>;

  // Schema browser
  schema: SchemaResponse | null;

  // UI state
  loading: {
    list: boolean;
    create: boolean;
    update: boolean;
    delete: boolean;
    query: boolean;
    schema: boolean;
    test: boolean;
  };
  error: string | null;
  connectionFormOpen: boolean;
  editingDatabaseId: string | null;
}

const initialState: DatabaseState = {
  databases: [],
  selectedDatabaseId: null,
  total: 0,
  page: 1,
  pageSize: 20,
  queryResult: null,
  queryHistory: [],
  schema: null,
  loading: {
    list: false,
    create: false,
    update: false,
    delete: false,
    query: false,
    schema: false,
    test: false,
  },
  error: null,
  connectionFormOpen: false,
  editingDatabaseId: null,
};

// Async thunks
export const fetchDatabases = createAsyncThunk(
  'databases/fetchList',
  async ({ page, pageSize }: { page?: number; pageSize?: number }, { rejectWithValue }) => {
    try {
      const response = await databaseApi.listDatabases(page, pageSize);
      return response;
    } catch (error) {
      return rejectWithValue((error as Error).message);
    }
  }
);

export const createDatabase = createAsyncThunk(
  'databases/create',
  async (data: DatabaseCreateRequest, { rejectWithValue }) => {
    try {
      const response = await databaseApi.createDatabase(data);
      return response;
    } catch (error) {
      return rejectWithValue((error as Error).message);
    }
  }
);

export const updateDatabase = createAsyncThunk(
  'databases/update',
  async ({ id, data }: { id: string; data: DatabaseUpdateRequest }, { rejectWithValue }) => {
    try {
      const response = await databaseApi.updateDatabase(id, data);
      return response;
    } catch (error) {
      return rejectWithValue((error as Error).message);
    }
  }
);

export const deleteDatabase = createAsyncThunk(
  'databases/delete',
  async (id: string, { rejectWithValue }) => {
    try {
      await databaseApi.deleteDatabase(id);
      return id;
    } catch (error) {
      return rejectWithValue((error as Error).message);
    }
  }
);

export const testConnection = createAsyncThunk(
  'databases/testConnection',
  async (id: string, { rejectWithValue }) => {
    try {
      const response = await databaseApi.testConnection(id);
      return response;
    } catch (error) {
      return rejectWithValue((error as Error).message);
    }
  }
);

export const executeQuery = createAsyncThunk(
  'databases/executeQuery',
  async (request: QueryRequest, { rejectWithValue }) => {
    try {
      const response = await databaseApi.executeQuery(request);
      return { ...response, databaseId: request.database_id, query: request.query };
    } catch (error) {
      return rejectWithValue((error as Error).message);
    }
  }
);

export const fetchSchema = createAsyncThunk(
  'databases/fetchSchema',
  async ({ id, type }: { id: string; type?: string }, { rejectWithValue }) => {
    try {
      const response = await databaseApi.getSchema(id, type);
      return response;
    } catch (error) {
      return rejectWithValue((error as Error).message);
    }
  }
);

// Slice
const databaseSlice = createSlice({
  name: 'databases',
  initialState,
  reducers: {
    selectDatabase: (state, action: PayloadAction<string | null>) => {
      state.selectedDatabaseId = action.payload;
      state.queryResult = null;
      state.schema = null;
    },
    openConnectionForm: (state, action: PayloadAction<string | null>) => {
      state.connectionFormOpen = true;
      state.editingDatabaseId = action.payload;
    },
    closeConnectionForm: (state) => {
      state.connectionFormOpen = false;
      state.editingDatabaseId = null;
    },
    clearQueryResult: (state) => {
      state.queryResult = null;
    },
    clearError: (state) => {
      state.error = null;
    },
    setPage: (state, action: PayloadAction<number>) => {
      state.page = action.payload;
    },
  },
  extraReducers: (builder) => {
    // Fetch databases
    builder
      .addCase(fetchDatabases.pending, (state) => {
        state.loading.list = true;
        state.error = null;
      })
      .addCase(fetchDatabases.fulfilled, (state, action) => {
        state.loading.list = false;
        state.databases = action.payload.databases;
        state.total = action.payload.total;
        state.page = action.payload.page;
        state.pageSize = action.payload.page_size;
      })
      .addCase(fetchDatabases.rejected, (state, action) => {
        state.loading.list = false;
        state.error = action.payload as string;
      });

    // Create database
    builder
      .addCase(createDatabase.pending, (state) => {
        state.loading.create = true;
        state.error = null;
      })
      .addCase(createDatabase.fulfilled, (state, action) => {
        state.loading.create = false;
        state.databases.unshift(action.payload);
        state.total += 1;
        state.connectionFormOpen = false;
      })
      .addCase(createDatabase.rejected, (state, action) => {
        state.loading.create = false;
        state.error = action.payload as string;
      });

    // Update database
    builder
      .addCase(updateDatabase.pending, (state) => {
        state.loading.update = true;
        state.error = null;
      })
      .addCase(updateDatabase.fulfilled, (state, action) => {
        state.loading.update = false;
        const index = state.databases.findIndex(db => db.id === action.payload.id);
        if (index !== -1) {
          state.databases[index] = action.payload;
        }
        state.connectionFormOpen = false;
        state.editingDatabaseId = null;
      })
      .addCase(updateDatabase.rejected, (state, action) => {
        state.loading.update = false;
        state.error = action.payload as string;
      });

    // Delete database
    builder
      .addCase(deleteDatabase.pending, (state) => {
        state.loading.delete = true;
        state.error = null;
      })
      .addCase(deleteDatabase.fulfilled, (state, action) => {
        state.loading.delete = false;
        state.databases = state.databases.filter(db => db.id !== action.payload);
        state.total -= 1;
        if (state.selectedDatabaseId === action.payload) {
          state.selectedDatabaseId = null;
        }
      })
      .addCase(deleteDatabase.rejected, (state, action) => {
        state.loading.delete = false;
        state.error = action.payload as string;
      });

    // Test connection
    builder
      .addCase(testConnection.pending, (state) => {
        state.loading.test = true;
      })
      .addCase(testConnection.fulfilled, (state, action) => {
        state.loading.test = false;
        const index = state.databases.findIndex(db => db.id === action.payload.id);
        if (index !== -1) {
          state.databases[index] = action.payload;
        }
      })
      .addCase(testConnection.rejected, (state, action) => {
        state.loading.test = false;
        state.error = action.payload as string;
      });

    // Execute query
    builder
      .addCase(executeQuery.pending, (state) => {
        state.loading.query = true;
        state.error = null;
      })
      .addCase(executeQuery.fulfilled, (state, action) => {
        state.loading.query = false;
        state.queryResult = action.payload;
        state.queryHistory.unshift({
          id: crypto.randomUUID(),
          databaseId: action.payload.databaseId,
          query: action.payload.query,
          executedAt: new Date().toISOString(),
          duration: action.payload.duration_ms,
          rowCount: action.payload.row_count,
        });
        // Keep only last 50 queries
        if (state.queryHistory.length > 50) {
          state.queryHistory = state.queryHistory.slice(0, 50);
        }
      })
      .addCase(executeQuery.rejected, (state, action) => {
        state.loading.query = false;
        state.error = action.payload as string;
      });

    // Fetch schema
    builder
      .addCase(fetchSchema.pending, (state) => {
        state.loading.schema = true;
      })
      .addCase(fetchSchema.fulfilled, (state, action) => {
        state.loading.schema = false;
        state.schema = action.payload;
      })
      .addCase(fetchSchema.rejected, (state, action) => {
        state.loading.schema = false;
        state.error = action.payload as string;
      });
  },
});

export const {
  selectDatabase,
  openConnectionForm,
  closeConnectionForm,
  clearQueryResult,
  clearError,
  setPage,
} = databaseSlice.actions;

export default databaseSlice.reducer;
```

### 2.3 API Service

**File: `src/services/api/databaseApi.ts`**

```typescript
import { apiClient } from './apiClient';
import type {
  Database,
  DatabaseCreateRequest,
  DatabaseUpdateRequest,
  DatabaseListResponse,
  QueryRequest,
  QueryResponse,
  SchemaResponse,
} from '../../types/database';

const BASE_PATH = '/api/v1/databases';

export const databaseApi = {
  /**
   * List all database connections
   */
  async listDatabases(page = 1, pageSize = 20): Promise<DatabaseListResponse> {
    const response = await apiClient.get<DatabaseListResponse>(
      `${BASE_PATH}?page=${page}&page_size=${pageSize}`
    );
    return response.data;
  },

  /**
   * Get a single database by ID
   */
  async getDatabase(id: string): Promise<Database> {
    const response = await apiClient.get<{ database: Database }>(`${BASE_PATH}/${id}`);
    return response.data.database;
  },

  /**
   * Create a new database connection
   */
  async createDatabase(data: DatabaseCreateRequest): Promise<Database> {
    const response = await apiClient.post<{ database: Database }>(BASE_PATH, data);
    return response.data.database;
  },

  /**
   * Update an existing database connection
   */
  async updateDatabase(id: string, data: DatabaseUpdateRequest): Promise<Database> {
    const response = await apiClient.put<{ database: Database }>(`${BASE_PATH}/${id}`, data);
    return response.data.database;
  },

  /**
   * Delete a database connection
   */
  async deleteDatabase(id: string): Promise<void> {
    await apiClient.delete(`${BASE_PATH}/${id}`);
  },

  /**
   * Test database connection
   */
  async testConnection(id: string): Promise<Database> {
    const response = await apiClient.post<{ database: Database }>(`${BASE_PATH}/${id}/test`);
    return response.data.database;
  },

  /**
   * Execute a SQL query
   */
  async executeQuery(request: QueryRequest): Promise<QueryResponse> {
    const response = await apiClient.post<QueryResponse>(`${BASE_PATH}/query`, request);
    return response.data;
  },

  /**
   * Get database schema information
   */
  async getSchema(id: string, type = 'table'): Promise<SchemaResponse> {
    const response = await apiClient.get<SchemaResponse>(
      `${BASE_PATH}/${id}/schema?type=${type}`
    );
    return response.data;
  },
};
```

### 2.4 Components Overview

**Component Structure:**

```
src/
├── components/
│   └── databases/
│       ├── DatabaseManager/
│       │   ├── index.tsx              # Main container
│       │   ├── DatabaseList.tsx       # List of connections with status
│       │   ├── DatabaseCard.tsx       # Individual database card
│       │   ├── ConnectionForm.tsx     # Create/edit form modal
│       │   └── ConnectionFormSchema.ts # Form validation schema
│       ├── QueryConsole/
│       │   ├── index.tsx              # Query console container
│       │   ├── QueryEditor.tsx        # SQL editor with syntax highlighting
│       │   ├── QueryResults.tsx       # Results table with pagination
│       │   ├── QueryHistory.tsx       # Query history sidebar
│       │   └── ExportButton.tsx       # Export to CSV/JSON
│       └── SchemaExplorer/
│           ├── index.tsx              # Schema browser container
│           ├── SchemaTree.tsx         # Tree view of schemas/tables
│           ├── TableInspector.tsx     # Table details panel
│           └── ColumnList.tsx         # Column listing
├── pages/
│   └── Database/
│       ├── DatabaseManagement.tsx     # Main database page
│       └── DatabaseQueryPage.tsx      # Query console page
└── hooks/
    └── useDatabase.ts                 # Custom hooks for database operations
```

### 2.5 Example Component: DatabaseList

**File: `src/components/databases/DatabaseManager/DatabaseList.tsx`**

```tsx
import React from 'react';
import { useSelector, useDispatch } from 'react-redux';
import { Database, Loader2, Plus, RefreshCw, Trash2, Edit, Play } from 'lucide-react';
import type { RootState, AppDispatch } from '../../../store';
import {
  selectDatabase,
  openConnectionForm,
  deleteDatabase,
  testConnection,
  fetchDatabases
} from '../../../store/slices/databaseSlice';
import type { Database as DatabaseType, DatabaseStatus } from '../../../types/database';

const statusColors: Record<DatabaseStatus, string> = {
  Connected: 'bg-green-100 text-green-800',
  Pending: 'bg-yellow-100 text-yellow-800',
  Failed: 'bg-red-100 text-red-800',
  Degraded: 'bg-orange-100 text-orange-800',
};

const databaseTypeIcons: Record<string, string> = {
  postgres: '🐘',
  mysql: '🐬',
  mariadb: '🦭',
  sqlserver: '🔷',
  sqlite: '📁',
};

export const DatabaseList: React.FC = () => {
  const dispatch = useDispatch<AppDispatch>();
  const { databases, selectedDatabaseId, loading, total, page, pageSize } = useSelector(
    (state: RootState) => state.databases
  );

  const handleSelect = (db: DatabaseType) => {
    dispatch(selectDatabase(db.id));
  };

  const handleEdit = (e: React.MouseEvent, id: string) => {
    e.stopPropagation();
    dispatch(openConnectionForm(id));
  };

  const handleDelete = async (e: React.MouseEvent, id: string) => {
    e.stopPropagation();
    if (confirm('Are you sure you want to delete this database connection?')) {
      await dispatch(deleteDatabase(id));
    }
  };

  const handleTest = async (e: React.MouseEvent, id: string) => {
    e.stopPropagation();
    await dispatch(testConnection(id));
  };

  const handleRefresh = () => {
    dispatch(fetchDatabases({ page, pageSize }));
  };

  return (
    <div className="space-y-4">
      {/* Header */}
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-gray-900">
          Database Connections ({total})
        </h2>
        <div className="flex items-center gap-2">
          <button
            onClick={handleRefresh}
            disabled={loading.list}
            className="p-2 text-gray-500 hover:text-gray-700 rounded-md hover:bg-gray-100"
            title="Refresh"
          >
            <RefreshCw className={`h-4 w-4 ${loading.list ? 'animate-spin' : ''}`} />
          </button>
          <button
            onClick={() => dispatch(openConnectionForm(null))}
            className="flex items-center gap-2 px-3 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700"
          >
            <Plus className="h-4 w-4" />
            Add Database
          </button>
        </div>
      </div>

      {/* Database List */}
      {loading.list && databases.length === 0 ? (
        <div className="flex items-center justify-center py-12">
          <Loader2 className="h-8 w-8 animate-spin text-blue-600" />
        </div>
      ) : databases.length === 0 ? (
        <div className="text-center py-12">
          <Database className="mx-auto h-12 w-12 text-gray-400" />
          <h3 className="mt-2 text-sm font-medium text-gray-900">No databases</h3>
          <p className="mt-1 text-sm text-gray-500">
            Get started by adding a database connection.
          </p>
          <button
            onClick={() => dispatch(openConnectionForm(null))}
            className="mt-4 inline-flex items-center gap-2 px-4 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700"
          >
            <Plus className="h-4 w-4" />
            Add Database
          </button>
        </div>
      ) : (
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
          {databases.map((db) => (
            <div
              key={db.id}
              onClick={() => handleSelect(db)}
              className={`p-4 border rounded-lg cursor-pointer transition-colors ${
                selectedDatabaseId === db.id
                  ? 'border-blue-500 bg-blue-50'
                  : 'border-gray-200 hover:border-gray-300 hover:bg-gray-50'
              }`}
            >
              {/* Card Header */}
              <div className="flex items-start justify-between">
                <div className="flex items-center gap-2">
                  <span className="text-2xl">{databaseTypeIcons[db.type] || '🗄️'}</span>
                  <div>
                    <h3 className="font-medium text-gray-900">{db.name}</h3>
                    <p className="text-sm text-gray-500">
                      {db.host}:{db.port}
                    </p>
                  </div>
                </div>
                <span className={`px-2 py-1 text-xs font-medium rounded-full ${statusColors[db.status]}`}>
                  {db.status}
                </span>
              </div>

              {/* Card Details */}
              <div className="mt-3 text-sm text-gray-600">
                <p>Database: <span className="font-mono">{db.database}</span></p>
                {db.readonly && (
                  <p className="text-orange-600">Read-only mode</p>
                )}
              </div>

              {/* Card Actions */}
              <div className="mt-4 flex items-center gap-2 pt-3 border-t border-gray-100">
                <button
                  onClick={(e) => handleTest(e, db.id)}
                  disabled={loading.test}
                  className="flex items-center gap-1 px-2 py-1 text-sm text-gray-600 hover:text-green-600 hover:bg-green-50 rounded"
                  title="Test connection"
                >
                  <Play className="h-3 w-3" />
                  Test
                </button>
                <button
                  onClick={(e) => handleEdit(e, db.id)}
                  className="flex items-center gap-1 px-2 py-1 text-sm text-gray-600 hover:text-blue-600 hover:bg-blue-50 rounded"
                  title="Edit"
                >
                  <Edit className="h-3 w-3" />
                  Edit
                </button>
                <button
                  onClick={(e) => handleDelete(e, db.id)}
                  disabled={loading.delete}
                  className="flex items-center gap-1 px-2 py-1 text-sm text-gray-600 hover:text-red-600 hover:bg-red-50 rounded"
                  title="Delete"
                >
                  <Trash2 className="h-3 w-3" />
                  Delete
                </button>
              </div>

              {/* Last Checked */}
              {db.last_checked && (
                <p className="mt-2 text-xs text-gray-400">
                  Last checked: {new Date(db.last_checked).toLocaleString()}
                </p>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
};
```

---

## Part 3: Integration Flow Diagram

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                        Complete Integration Flow                                │
├────────────────────────────────────────────────────────────────────────────────┤
│                                                                                 │
│  ┌─────────────────┐                                                           │
│  │  Aether Frontend│                                                           │
│  │    (React)      │                                                           │
│  └────────┬────────┘                                                           │
│           │                                                                     │
│           │ 1. User creates database connection                                │
│           │    POST /api/v1/databases                                          │
│           ▼                                                                     │
│  ┌─────────────────┐     2a. Store metadata                                    │
│  │  Aether Backend │─────────────────────────┐                                 │
│  │     (Go)        │                          │                                 │
│  └────────┬────────┘                          ▼                                 │
│           │                          ┌─────────────────┐                       │
│           │                          │     Neo4j       │                       │
│           │ 2b. Create K8s resources │  (Graph DB)     │                       │
│           │                          └─────────────────┘                       │
│           ▼                                                                     │
│  ┌─────────────────┐                                                           │
│  │  Kubernetes API │                                                           │
│  └────────┬────────┘                                                           │
│           │                                                                     │
│           │ Creates:                                                           │
│           │ - Secret (credentials)                                             │
│           │ - Database CR                                                      │
│           ▼                                                                     │
│  ┌─────────────────┐     3. Watches Database CRs                               │
│  │  DBHub Operator │◀─────────────────────────────────────────────────────┐   │
│  │  (Controller)   │                                                       │   │
│  └────────┬────────┘                                                       │   │
│           │                                                                 │   │
│           │ 4. On new/updated Database CR:                                  │   │
│           │    - Fetch credentials from Secret                              │   │
│           │    - Test database connection                                   │   │
│           │    - Update CR status                                           │   │
│           │    - Regenerate DBHubInstance config                            │   │
│           │    - Restart/reload DBHub pods                                  │   │
│           ▼                                                                 │   │
│  ┌─────────────────┐     ┌─────────────────┐                               │   │
│  │ DBHubInstance   │────▶│   DBHub Pod     │                               │   │
│  │  (Deployment)   │     │  (MCP Server)   │                               │   │
│  └─────────────────┘     └────────┬────────┘                               │   │
│                                   │                                         │   │
│                                   │ 5. MCP tools available                  │   │
│                                   │    - execute_sql                        │   │
│                                   │    - search_objects                     │   │
│                                   ▼                                         │   │
│                          ┌─────────────────┐                               │   │
│                          │  User Databases │                               │   │
│                          │  (PostgreSQL,   │                               │   │
│                          │   MySQL, etc.)  │                               │   │
│                          └─────────────────┘                               │   │
│                                                                             │   │
│  ┌─────────────────────────────────────────────────────────────────────────┤   │
│  │                        Status Update Flow                               │   │
│  ├─────────────────────────────────────────────────────────────────────────┤   │
│  │                                                                         │   │
│  │  6. Operator updates Database CR status                                 │   │
│  │     (Connected/Failed/Degraded)                                         │   │
│  │                                  │                                      │   │
│  │                                  ▼                                      │   │
│  │  7. Aether Backend watches/polls CR status ─────────────────────────────┘   │
│  │     - Updates Neo4j node status                                             │
│  │                                  │                                          │
│  │                                  ▼                                          │
│  │  8. Frontend polls GET /api/v1/databases/{id}                               │
│  │     - Displays updated status to user                                       │
│  │                                                                             │
│  └─────────────────────────────────────────────────────────────────────────────┘
│                                                                                 │
│  ┌─────────────────────────────────────────────────────────────────────────────┐
│  │                        Query Execution Flow                                 │
│  ├─────────────────────────────────────────────────────────────────────────────┤
│  │                                                                             │
│  │  1. User executes query in frontend                                         │
│  │     POST /api/v1/databases/query                                            │
│  │                     │                                                       │
│  │                     ▼                                                       │
│  │  2. Aether Backend validates query & permissions                            │
│  │     - Check readonly mode                                                   │
│  │     - Check user has access to database                                     │
│  │                     │                                                       │
│  │                     ▼                                                       │
│  │  3. Backend calls DBHub MCP endpoint                                        │
│  │     POST http://dbhub:8080/mcp                                              │
│  │     {method: "tools/call", params: {name: "execute_sql", ...}}              │
│  │                     │                                                       │
│  │                     ▼                                                       │
│  │  4. DBHub executes query against database                                   │
│  │     - Applies row limits                                                    │
│  │     - Applies timeouts                                                      │
│  │                     │                                                       │
│  │                     ▼                                                       │
│  │  5. Results returned to frontend                                            │
│  │     {columns: [...], rows: [...], duration_ms: ...}                         │
│  │                                                                             │
│  └─────────────────────────────────────────────────────────────────────────────┘
│                                                                                 │
└────────────────────────────────────────────────────────────────────────────────┘
```

---

## Part 4: Security Considerations

### 4.1 Credential Management

1. **Frontend**:
   - Never stores or displays passwords
   - Password fields use `type="password"`
   - Credentials sent over HTTPS only

2. **Backend**:
   - Credentials stored only in Kubernetes Secrets
   - Neo4j stores only Secret references, never credentials
   - All credential operations are audited

3. **Kubernetes**:
   - Secrets encrypted at rest (etcd encryption)
   - RBAC restricts Secret access to operator only
   - External Secrets Operator integration for production

### 4.2 Multi-Tenancy Isolation

```go
// All database queries MUST filter by tenant_id
query := `
    MATCH (d:Database {tenant_id: $tenant_id})
    WHERE d.id = $id
    RETURN d
`

// Space context middleware enforces tenant isolation
func RequireSpaceContext() gin.HandlerFunc {
    return func(c *gin.Context) {
        tenantID := c.Get("tenant_id")
        if tenantID == nil || tenantID == "" {
            c.JSON(403, errors.Forbidden("Space context required"))
            c.Abort()
            return
        }
        c.Next()
    }
}
```

### 4.3 Query Safety

1. **Read-only mode** enforced per-database
2. **Row limits** prevent resource exhaustion (default: 1000)
3. **Query timeouts** prevent long-running queries (default: 15s)
4. **Write query detection** blocks INSERT/UPDATE/DELETE when readonly

### 4.4 Audit Logging

```go
// Log all database operations
log.Info("Database query executed",
    zap.String("user_id", userID),
    zap.String("database_id", dbID),
    zap.String("query_hash", hashQuery(query)), // Hash for privacy
    zap.Int("row_count", result.RowCount),
    zap.Int64("duration_ms", result.Duration),
)
```

---

## Part 5: Implementation Phases

### Phase 1: Backend API (Week 1-2)
- [ ] Create database models
- [ ] Implement DatabaseService
- [ ] Implement DBHubService (MCP client)
- [ ] Create DatabaseHandler with routes
- [ ] Add configuration for DBHub
- [ ] Write unit tests

### Phase 2: Kubernetes Integration (Week 2-3)
- [ ] Add Kubernetes client to backend
- [ ] Implement CRD create/update/delete
- [ ] Implement Secret management
- [ ] Add status sync from K8s to Neo4j
- [ ] Write integration tests

### Phase 3: Frontend (Week 3-4)
- [ ] Create TypeScript types
- [ ] Implement Redux slice
- [ ] Create API service
- [ ] Build DatabaseList component
- [ ] Build ConnectionForm component
- [ ] Build QueryConsole component
- [ ] Build SchemaExplorer component

### Phase 4: Testing & Documentation (Week 4-5)
- [ ] End-to-end testing
- [ ] Performance testing
- [ ] Security review
- [ ] API documentation
- [ ] User documentation

---

## Appendix: API Reference

### Database Management Endpoints

| Method | Path | Description |
|--------|------|-------------|
| POST | /api/v1/databases | Create database connection |
| GET | /api/v1/databases | List databases |
| GET | /api/v1/databases/:id | Get database |
| PUT | /api/v1/databases/:id | Update database |
| DELETE | /api/v1/databases/:id | Delete database |
| POST | /api/v1/databases/:id/test | Test connection |
| GET | /api/v1/databases/:id/schema | Get schema info |
| POST | /api/v1/databases/query | Execute SQL query |

### Response Codes

| Code | Description |
|------|-------------|
| 200 | Success |
| 201 | Created |
| 204 | No Content (delete) |
| 400 | Validation error |
| 401 | Unauthorized |
| 403 | Forbidden |
| 404 | Not found |
| 409 | Conflict |
| 500 | Internal error |
| 503 | Service unavailable |
