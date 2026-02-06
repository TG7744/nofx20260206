#!/bin/bash
#
# NOFX Dev/Test Installation Script
# https://github.com/TG7744/nofx20260206
#
# Usage (dev/test):
#   curl -fsSL https://raw.githubusercontent.com/TG7744/nofx20260206/dev/install-dev.sh | bash
# Or with custom directory:
#   curl -fsSL https://raw.githubusercontent.com/TG7744/nofx20260206/dev/install-dev.sh | bash -s -- /opt/nofx-dev
#

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

INSTALL_DIR="${1:-$HOME/nofx-dev}"
COMPOSE_FILE="docker-compose.dev.yml"
GITHUB_RAW="https://raw.githubusercontent.com/TG7744/nofx20260206/dev"

echo -e "${BLUE}"
echo "============================================================"
echo "                     NOFX Dev Install                       "
echo "============================================================"
echo -e "${NC}"

check_docker() {
    echo -e "${YELLOW}Checking Docker...${NC}"
    if ! command -v docker &> /dev/null; then
        echo -e "${RED}Error: Docker is not installed.${NC}"
        exit 1
    fi
    if ! docker info &> /dev/null; then
        echo -e "${RED}Error: Docker daemon is not running.${NC}"
        exit 1
    fi
    if docker compose version &> /dev/null; then
        COMPOSE_CMD="docker compose"
    elif command -v docker-compose &> /dev/null; then
        COMPOSE_CMD="docker-compose"
    else
        echo -e "${RED}Error: Docker Compose is not available.${NC}"
        exit 1
    fi
    echo -e "${GREEN}✓ Docker ready${NC}"
}

setup_directory() {
    echo -e "${YELLOW}Setting up installation directory: ${INSTALL_DIR}${NC}"
    mkdir -p "$INSTALL_DIR"
    cd "$INSTALL_DIR"
    echo -e "${GREEN}✓ Directory ready${NC}"
}

download_files() {
    echo -e "${YELLOW}Downloading compose file...${NC}"
    curl -fsSL "$GITHUB_RAW/$COMPOSE_FILE" -o docker-compose.yml
    echo -e "${GREEN}✓ Compose downloaded${NC}"
}

generate_env() {
    echo -e "${YELLOW}Generating .env (dev/test)...${NC}"
    if [ -f ".env" ]; then
        echo -e "${GREEN}✓ .env exists, skip${NC}"
        return
    fi
    JWT_SECRET=$(openssl rand -base64 32)
    DATA_ENCRYPTION_KEY=$(openssl rand -base64 32)
    RSA_PRIVATE_KEY=$(openssl genrsa 2048 2>/dev/null | tr '\n' '\\' | sed 's/\\\\/\\\\n/g' | sed 's/\\\\n$//')
    cat > .env << EOF
NOFX_BACKEND_PORT=8080
NOFX_FRONTEND_PORT=3000
TZ=Asia/Shanghai
JWT_SECRET=${JWT_SECRET}
DATA_ENCRYPTION_KEY=${DATA_ENCRYPTION_KEY}
RSA_PRIVATE_KEY=${RSA_PRIVATE_KEY}
EOF
    echo -e "${GREEN}✓ .env generated${NC}"
}

pull_images() {
    echo -e "${YELLOW}Pulling dev/test images...${NC}"
    $COMPOSE_CMD pull
    echo -e "${GREEN}✓ Images pulled${NC}"
}

start_services() {
    echo -e "${YELLOW}Starting services...${NC}"
    $COMPOSE_CMD up -d
    echo -e "${GREEN}✓ Services started${NC}"
}

print_success() {
    local ip=$(curl -s --max-time 3 ifconfig.me 2>/dev/null || echo "127.0.0.1")
    echo ""
    echo -e "${GREEN}Dev/Test environment ready!${NC}"
    echo -e "  Web: http://${ip}:3000"
    echo -e "  API: http://${ip}:8080"
}

main() {
    check_docker
    setup_directory
    download_files
    generate_env
    pull_images
    start_services
    print_success
}

main
