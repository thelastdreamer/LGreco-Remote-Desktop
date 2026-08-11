#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

# Colors
RED='\033[0;31m'; GREEN='\033[0;32m'; BLUE='\033[0;34m'; NC='\033[0m'

echo -e "${BLUE}====================================${NC}"
echo -e "${BLUE}  Remote Desktop - One-Click Deploy${NC}"
echo -e "${BLUE}====================================${NC}"
echo ""

# 1. Check prerequisites
echo -e "${GREEN}[1/6] Checking prerequisites...${NC}"
command -v docker >/dev/null 2>&1 || { echo -e "${RED}Docker required. Install: curl -fsSL https://get.docker.com | sh${NC}"; exit 1; }
command -v docker compose >/dev/null 2>&1 && COMPOSE_CMD="docker compose" || {
    docker compose version >/dev/null 2>&1 && COMPOSE_CMD="docker compose" || COMPOSE_CMD="docker-compose"
}
echo "  Docker: OK"
echo "  Docker Compose: OK"

# 2. Load / Create .env
echo -e "${GREEN}[2/6] Configuring environment...${NC}"
if [ ! -f "$PROJECT_DIR/.env" ]; then
    cp "$PROJECT_DIR/.env.example" "$PROJECT_DIR/.env"

    # Generate random secrets
    JWT_SECRET=$(openssl rand -hex 32 2>/dev/null || head -c 32 /dev/urandom | xxd -p)
    DB_PASS=$(openssl rand -hex 16 2>/dev/null || head -c 16 /dev/urandom | xxd -p)
    TURN_PASS=$(openssl rand -hex 16 2>/dev/null || head -c 16 /dev/urandom | xxd -p)

    sed -i "s/change_this_password/$DB_PASS/g" "$PROJECT_DIR/.env"
    sed -i "s/change-me-to-a-random-64-character-string-at-least-32-bytes/$JWT_SECRET/g" "$PROJECT_DIR/.env"
    sed -i "s/change_this_turn_password/$TURN_PASS/g" "$PROJECT_DIR/.env"
    echo "  Generated .env with random secrets"
else
    echo "  Using existing .env"
fi

# 3. Build Docker images
echo -e "${GREEN}[3/6] Building Docker images...${NC}"
cd "$PROJECT_DIR"
docker build -t rd-desktop:latest -f desktop-container/Dockerfile desktop-container/
docker build -t rd-relay:latest -f relay-container/Dockerfile relay-container/
docker build -t rd-server:latest -f server/Dockerfile server/
echo "  Images built: OK"

# 4. Create Docker network
echo -e "${GREEN}[4/6] Creating Docker network...${NC}"
docker network create rd-network 2>/dev/null || echo "  Network already exists"

# 5. Start services
echo -e "${GREEN}[5/6] Starting services...${NC}"
$COMPOSE_CMD up -d

# 6. Wait and verify
echo -e "${GREEN}[6/6] Verifying deployment...${NC}"
sleep 5
if curl -sf http://localhost:8080/health >/dev/null 2>&1; then
    echo -e "${GREEN}  Server is healthy!${NC}"
else
    echo -e "${RED}  Server check failed. Run: docker compose logs api${NC}"
    exit 1
fi

echo ""
echo -e "${BLUE}====================================${NC}"
echo -e "${GREEN}  Deployment Complete!${NC}"
echo -e "${BLUE}====================================${NC}"
echo ""
echo "  API Server:     http://localhost:8080"
echo "  API Health:     http://localhost:8080/health"
echo "  TURN Server:    localhost:3478 (UDP+TCP)"
echo "  TURN Relay:     UDP 49152-49201 (50 ports)"
echo ""
echo "  Register a user:"
echo "  curl -X POST http://localhost:8080/api/register \\"
echo "    -H 'Content-Type: application/json' \\"
echo "    -d '{\"username\":\"admin\",\"email\":\"admin@local\",\"password\":\"admin123\"}'"
echo ""
echo "  View logs:  docker compose logs -f"
echo "  Stop:       docker compose down"
echo ""
