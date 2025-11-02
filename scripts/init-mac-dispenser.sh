#!/bin/bash
# Rock MAC Address Dispenser - Initialization Script
# This script sets up SQLite database and dependencies for rock-mac tool
# Can be run on any macOS or Linux machine for reproducible setup

set -e  # Exit on error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "=========================================="
echo "Rock MAC Dispenser - Setup Script"
echo "=========================================="
echo ""

# 1. Check Operating System
OS="$(uname -s)"
case "${OS}" in
    Linux*)     OS_TYPE=Linux;;
    Darwin*)    OS_TYPE=Mac;;
    *)          echo -e "${RED}✗ Unsupported OS: ${OS}${NC}"; exit 1;;
esac
echo -e "${GREEN}✓ Operating System: ${OS_TYPE}${NC}"

# 2. Check/Install SQLite
echo ""
echo "Checking SQLite installation..."
if command -v sqlite3 &> /dev/null; then
    SQLITE_VERSION=$(sqlite3 --version | awk '{print $1}')
    echo -e "${GREEN}✓ SQLite found: ${SQLITE_VERSION}${NC}"
else
    echo -e "${YELLOW}SQLite not found. Installing...${NC}"

    if [[ "$OS_TYPE" == "Mac" ]]; then
        # Check if Homebrew is installed
        if command -v brew &> /dev/null; then
            brew install sqlite3
        else
            echo -e "${RED}✗ Homebrew not found. Please install Homebrew first:${NC}"
            echo "  /bin/bash -c \"\$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)\""
            exit 1
        fi
    elif [[ "$OS_TYPE" == "Linux" ]]; then
        # Detect Linux distribution
        if [ -f /etc/debian_version ]; then
            sudo apt-get update && sudo apt-get install -y sqlite3
        elif [ -f /etc/redhat-release ]; then
            sudo yum install -y sqlite
        else
            echo -e "${RED}✗ Unsupported Linux distribution${NC}"
            echo "  Please install SQLite manually: sudo <package-manager> install sqlite3"
            exit 1
        fi
    fi

    # Verify installation
    if command -v sqlite3 &> /dev/null; then
        SQLITE_VERSION=$(sqlite3 --version | awk '{print $1}')
        echo -e "${GREEN}✓ SQLite installed: ${SQLITE_VERSION}${NC}"
    else
        echo -e "${RED}✗ SQLite installation failed${NC}"
        exit 1
    fi
fi

# 3. Create database directory
echo ""
echo "Creating database directory..."
DB_DIR="$HOME/.rock"
if [ ! -d "$DB_DIR" ]; then
    mkdir -p "$DB_DIR"
    echo -e "${GREEN}✓ Created directory: ${DB_DIR}${NC}"
else
    echo -e "${GREEN}✓ Directory exists: ${DB_DIR}${NC}"
fi

# 4. Initialize database with schema
echo ""
echo "Initializing MAC dispenser database..."
DB_PATH="$DB_DIR/mac-dispenser.db"

# Create backup if database exists
if [ -f "$DB_PATH" ]; then
    BACKUP_PATH="${DB_PATH}.backup.$(date +%Y%m%d_%H%M%S)"
    cp "$DB_PATH" "$BACKUP_PATH"
    echo -e "${YELLOW}⚠ Existing database backed up to: ${BACKUP_PATH}${NC}"
fi

# Create database schema
sqlite3 "$DB_PATH" <<EOF
-- MAC Address Allocations table
CREATE TABLE IF NOT EXISTS mac_allocations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    mac_address TEXT UNIQUE NOT NULL,
    pool TEXT NOT NULL,
    device_id TEXT,
    device_type TEXT,
    metadata TEXT,  -- JSON string for flexible metadata
    allocated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    released_at TIMESTAMP,
    last_seen TIMESTAMP,
    status TEXT DEFAULT 'active' CHECK(status IN ('active', 'released', 'reserved'))
);

-- Create indexes for performance
CREATE INDEX IF NOT EXISTS idx_mac_address ON mac_allocations(mac_address);
CREATE INDEX IF NOT EXISTS idx_pool ON mac_allocations(pool);
CREATE INDEX IF NOT EXISTS idx_device_id ON mac_allocations(device_id);
CREATE INDEX IF NOT EXISTS idx_status ON mac_allocations(status);
CREATE INDEX IF NOT EXISTS idx_allocated_at ON mac_allocations(allocated_at);

-- Configuration table for pools
CREATE TABLE IF NOT EXISTS pools (
    name TEXT PRIMARY KEY,
    range_start TEXT NOT NULL,
    range_end TEXT NOT NULL,
    description TEXT,
    auto_release_days INTEGER DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Counter table for sequential allocation
CREATE TABLE IF NOT EXISTS counters (
    pool TEXT PRIMARY KEY,
    last_allocated TEXT NOT NULL,
    total_allocated INTEGER DEFAULT 0,
    total_released INTEGER DEFAULT 0,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Audit log table for tracking changes
CREATE TABLE IF NOT EXISTS audit_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    action TEXT NOT NULL,
    mac_address TEXT,
    pool TEXT,
    device_id TEXT,
    user TEXT,
    details TEXT,  -- JSON string for additional details
    timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Create index for audit log
CREATE INDEX IF NOT EXISTS idx_audit_timestamp ON audit_log(timestamp);

-- Initialize default pools from configuration
INSERT OR IGNORE INTO pools (name, range_start, range_end, description, auto_release_days) VALUES
    ('production', '00:00:01', '00:ff:ff', 'Production nodes', 0),
    ('development', '01:00:00', '01:ff:ff', 'Development/test nodes', 7),
    ('experiment', '02:00:00', '02:ff:ff', 'Experiment environments', 1),
    ('vultr', '03:00:00', '03:ff:ff', 'Vultr cloud instances', 30),
    ('docker', '04:00:00', '04:ff:ff', 'Docker containers', 1),
    ('kubernetes', '05:00:00', '05:ff:ff', 'Kubernetes pods', 1),
    ('reserved', 'ff:00:00', 'ff:ff:ff', 'Reserved for special use', 0);

-- Initialize counters for each pool
INSERT OR IGNORE INTO counters (pool, last_allocated, total_allocated, total_released) VALUES
    ('production', '00:00:00', 0, 0),
    ('development', '00:ff:ff', 0, 0),
    ('experiment', '01:ff:ff', 0, 0),
    ('vultr', '02:ff:ff', 0, 0),
    ('docker', '03:ff:ff', 0, 0),
    ('kubernetes', '04:ff:ff', 0, 0),
    ('reserved', 'fe:ff:ff', 0, 0);

-- Create view for active allocations
CREATE VIEW IF NOT EXISTS active_allocations AS
SELECT
    mac_address,
    pool,
    device_id,
    device_type,
    allocated_at,
    (julianday('now') - julianday(allocated_at)) as days_allocated
FROM mac_allocations
WHERE status = 'active';

-- Create view for pool statistics
CREATE VIEW IF NOT EXISTS pool_stats AS
SELECT
    p.name as pool,
    p.description,
    COUNT(CASE WHEN m.status = 'active' THEN 1 END) as active_count,
    COUNT(CASE WHEN m.status = 'released' THEN 1 END) as released_count,
    COUNT(CASE WHEN m.status = 'reserved' THEN 1 END) as reserved_count,
    c.total_allocated,
    c.total_released
FROM pools p
LEFT JOIN mac_allocations m ON p.name = m.pool
LEFT JOIN counters c ON p.name = c.pool
GROUP BY p.name;

-- Database info
.tables
EOF

if [ $? -eq 0 ]; then
    echo -e "${GREEN}✓ Database initialized: ${DB_PATH}${NC}"
else
    echo -e "${RED}✗ Database initialization failed${NC}"
    exit 1
fi

# 5. Check Go installation (needed for rock-mac tool)
echo ""
echo "Checking Go installation..."
if command -v go &> /dev/null; then
    GO_VERSION=$(go version | awk '{print $3}')
    echo -e "${GREEN}✓ Go found: ${GO_VERSION}${NC}"
else
    echo -e "${YELLOW}⚠ Go not found. Rock-mac tool requires Go 1.21+${NC}"
    echo "  Install Go from: https://golang.org/dl/"
fi

# 6. Create test script for verification
echo ""
echo "Creating test script..."
TEST_SCRIPT="$HOME/.rock/test-mac-dispenser.sh"
cat > "$TEST_SCRIPT" <<'TESTEOF'
#!/bin/bash
# Test script for MAC dispenser database

DB_PATH="$HOME/.rock/mac-dispenser.db"

echo "Testing MAC dispenser database..."
echo ""

# Test 1: Check tables
echo "1. Checking tables..."
sqlite3 "$DB_PATH" ".tables"
echo ""

# Test 2: Check pools
echo "2. Available pools:"
sqlite3 "$DB_PATH" -column -header "SELECT name, description, auto_release_days FROM pools;"
echo ""

# Test 3: Check pool statistics
echo "3. Pool statistics:"
sqlite3 "$DB_PATH" -column -header "SELECT * FROM pool_stats;"
echo ""

# Test 4: Simulate MAC allocation
echo "4. Simulating MAC allocation to production pool..."
sqlite3 "$DB_PATH" <<SQL
INSERT INTO mac_allocations (mac_address, pool, device_id, device_type, metadata)
VALUES ('a4:58:0f:00:00:01', 'production', 'test-node-01', 'qemu-vm', '{"test": true}');

UPDATE counters
SET last_allocated = '00:00:01', total_allocated = total_allocated + 1, updated_at = CURRENT_TIMESTAMP
WHERE pool = 'production';

INSERT INTO audit_log (action, mac_address, pool, device_id, user, details)
VALUES ('allocate', 'a4:58:0f:00:00:01', 'production', 'test-node-01', 'test-script', '{"reason": "test"}');
SQL

# Test 5: Query allocated MAC
echo "5. Query allocated MACs:"
sqlite3 "$DB_PATH" -column -header "SELECT mac_address, pool, device_id, status FROM mac_allocations;"
echo ""

# Test 6: Check audit log
echo "6. Recent audit log entries:"
sqlite3 "$DB_PATH" -column -header "SELECT action, mac_address, pool, timestamp FROM audit_log ORDER BY timestamp DESC LIMIT 5;"
echo ""

echo "✅ Database test complete!"
TESTEOF

chmod +x "$TEST_SCRIPT"
echo -e "${GREEN}✓ Test script created: ${TEST_SCRIPT}${NC}"

# 7. Display summary
echo ""
echo "=========================================="
echo -e "${GREEN}Setup Complete!${NC}"
echo "=========================================="
echo ""
echo "Database Location: ${DB_PATH}"
echo "Test Script: ${TEST_SCRIPT}"
echo ""
echo "Next steps:"
echo "1. Run test script: ${TEST_SCRIPT}"
echo "2. Build rock-mac tool: cd rock-mac && go build"
echo "3. Use rock-mac CLI to manage MAC addresses"
echo ""
echo "SQLite CLI access:"
echo "  sqlite3 ${DB_PATH}"
echo ""
echo "Example queries:"
echo "  .tables                                    # List all tables"
echo "  SELECT * FROM pools;                       # View all pools"
echo "  SELECT * FROM pool_stats;                  # View statistics"
echo "  SELECT * FROM active_allocations;          # View active MACs"
echo ""