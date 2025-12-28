# Synology File Cache

Synology Drive HTTP API를 사용해 중요 파일들을 로컬에 프리패치하고, NAS 오프라인 시에도 파일 서빙이 가능하도록 하는 Go 기반 캐싱 서비스입니다.

## 주요 기능

- **우선순위 기반 프리패치**: 공유/즐겨찾기/라벨/최근 수정 파일을 자동으로 로컬에 캐싱
- **스마트 디스크 관리**: 설정 가능한 디스크 사용량 제한과 우선순위+LRU 기반 자동 삭제
- **Synology 호환성**: Synology 공유 링크 토큰을 그대로 사용하여 기존 링크 유지
- **자동 캐시 갱신**: 파일 수정 시간(mtime) 기반 자동 캐시 무효화
- **투명한 프록시**: NAS 온라인 시에는 직접 접근, 오프라인 시에는 캐시에서 서빙

## 빠른 시작

### Docker Compose (권장)

```bash
git clone https://github.com/VertexToEdge/synology-file-cache.git
cd synology-file-cache

# docker-compose.yaml에서 환경변수 수정 후 실행
docker compose up -d
```

### 바이너리 직접 실행

```bash
# 빌드
go build -o synology-file-cache ./cmd/synology-file-cache

# 설정 파일 사용
./synology-file-cache -config config.yaml

# 또는 환경변수만 사용
SFC_SYNOLOGY_BASE_URL=https://nas.local:5001 \
SFC_SYNOLOGY_USERNAME=admin \
SFC_SYNOLOGY_PASSWORD=password \
./synology-file-cache
```

## 설정

설정은 환경변수 또는 YAML 파일로 지정합니다. 환경변수가 설정 파일보다 우선합니다.

### 필수 설정

| 환경변수 | 설명 |
|---------|------|
| `SFC_SYNOLOGY_BASE_URL` | Synology NAS URL (예: `https://nas.local:5001`) |
| `SFC_SYNOLOGY_USERNAME` | 사용자명 |
| `SFC_SYNOLOGY_PASSWORD` | 비밀번호 |

### 주요 설정

| 환경변수 | 기본값 | 설명 |
|---------|-------|------|
| `SFC_CACHE_ROOT_DIR` | `/data` | 캐시 저장 경로 |
| `SFC_CACHE_MAX_SIZE_GB` | `50` | 최대 캐시 크기 (GB) |
| `SFC_CACHE_MAX_DISK_USAGE_PERCENT` | `50` | 디스크 사용률 제한 (%) |
| `SFC_CACHE_CONCURRENT_DOWNLOADS` | `3` | 동시 다운로드 수 (1-10) |
| `SFC_SYNC_FULL_SCAN_INTERVAL` | `1h` | 전체 동기화 주기 |
| `SFC_SYNC_INCREMENTAL_INTERVAL` | `1m` | 증분 동기화 주기 |
| `SFC_LOGGING_LEVEL` | `info` | 로그 레벨 (debug/info/warn/error) |

<details>
<summary><b>전체 환경변수 목록</b></summary>

| 환경변수 | 기본값 | 설명 |
|---------|-------|------|
| **Synology 연결** |||
| `SFC_SYNOLOGY_BASE_URL` | - | Synology NAS URL |
| `SFC_SYNOLOGY_USERNAME` | - | 사용자명 |
| `SFC_SYNOLOGY_PASSWORD` | - | 비밀번호 |
| `SFC_SYNOLOGY_SKIP_TLS_VERIFY` | `false` | TLS 인증서 검증 무시 |
| **캐시** |||
| `SFC_CACHE_ROOT_DIR` | `/data` | 캐시 저장 경로 |
| `SFC_CACHE_MAX_SIZE_GB` | `50` | 최대 캐시 크기 (GB) |
| `SFC_CACHE_MAX_DISK_USAGE_PERCENT` | `50` | 디스크 사용률 제한 (%) |
| `SFC_CACHE_RECENT_MODIFIED_DAYS` | `30` | 최근 수정 파일 기준 (일) |
| `SFC_CACHE_RECENT_ACCESSED_DAYS` | `30` | 최근 접근 파일 기준 (일) |
| `SFC_CACHE_CONCURRENT_DOWNLOADS` | `3` | 동시 다운로드 수 (1-10) |
| `SFC_CACHE_EVICTION_INTERVAL` | `30s` | 캐시 정리 주기 |
| `SFC_CACHE_BUFFER_SIZE_MB` | `8` | 다운로드 버퍼 크기 (MB) |
| `SFC_CACHE_STALE_TASK_TIMEOUT` | `30m` | 정체된 작업 타임아웃 |
| `SFC_CACHE_PROGRESS_UPDATE_INTERVAL` | `10s` | 진행률 업데이트 주기 |
| `SFC_CACHE_MAX_DOWNLOAD_RETRIES` | `3` | 최대 다운로드 재시도 횟수 |
| **동기화** |||
| `SFC_SYNC_FULL_SCAN_INTERVAL` | `1h` | 전체 스캔 주기 |
| `SFC_SYNC_INCREMENTAL_INTERVAL` | `1m` | 증분 동기화 주기 |
| `SFC_SYNC_PREFETCH_INTERVAL` | `30s` | 프리패치 실행 주기 |
| `SFC_SYNC_PAGE_SIZE` | `200` | API 페이지 크기 |
| **HTTP 서버** |||
| `SFC_HTTP_BIND_ADDR` | `0.0.0.0:8080` | 바인딩 주소 |
| `SFC_HTTP_ENABLE_ADMIN_BROWSER` | `false` | Admin 브라우저 활성화 |
| `SFC_HTTP_READ_TIMEOUT` | `30s` | HTTP 읽기 타임아웃 |
| `SFC_HTTP_WRITE_TIMEOUT` | `30s` | HTTP 쓰기 타임아웃 |
| `SFC_HTTP_IDLE_TIMEOUT` | `60s` | HTTP 유휴 타임아웃 |
| **로깅** |||
| `SFC_LOGGING_LEVEL` | `info` | 로그 레벨 (debug/info/warn/error) |
| `SFC_LOGGING_FORMAT` | `json` | 로그 포맷 (json/text) |
| **데이터베이스** |||
| `SFC_DATABASE_PATH` | `{root_dir}/cache.db` | 데이터베이스 경로 |
| `SFC_DATABASE_CACHE_SIZE_MB` | `64` | SQLite 캐시 크기 (MB) |
| `SFC_DATABASE_BUSY_TIMEOUT_MS` | `5000` | SQLite busy 타임아웃 (ms) |

</details>

### 캐시 우선순위

파일은 다음 우선순위로 캐싱됩니다 (낮은 숫자 = 높은 우선순위):

| 우선순위 | 유형 | 설명 |
|---------|------|------|
| 1 | 공유된 파일 | 외부 공유 링크가 있는 파일 |
| 2 | 즐겨찾기/라벨 | Star 표시 또는 라벨이 붙은 파일 |
| 3 | 최근 수정 | 설정된 기간 내 수정된 파일 |
| 4 | 최근 접근 | 설정된 기간 내 접근된 파일 |
| 5 | 기본값 | 기타 파일 |

- **캐싱 순서**: 우선순위 오름차순 → 파일 크기 오름차순
- **삭제 순서**: 우선순위 내림차순 → LRU (오래된 파일 먼저)

## API 엔드포인트

| 엔드포인트 | 설명 |
|-----------|------|
| `GET /health` | 헬스체크 |
| `GET /f/{token}` | 공유 토큰으로 파일 다운로드 |
| `GET /d/s/{token}` | Synology 형식 호환 |
| `GET /d/s/{token}/{filename}` | 파일명 포함 경로 |
| `GET /debug/stats` | 캐시 통계 (JSON) |
| `GET /debug/files` | 캐시된 파일 목록 (JSON) |

## 프록시 연동

NAS 오프라인 시 자동으로 캐시에서 서빙하려면 리버스 프록시의 failover 기능을 활용합니다.

<details>
<summary><b>Traefik 설정 예제</b></summary>

```yaml
http:
  routers:
    synology-drive:
      rule: "Host(`drive.example.com`)"
      service: synology-drive-service

  services:
    synology-drive-service:
      failover:
        service: synology-nas
        fallback: synology-cache
        healthCheck:
          path: /health
          interval: "10s"

    synology-nas:
      loadBalancer:
        servers:
          - url: "https://nas.local:5001"

    synology-cache:
      loadBalancer:
        servers:
          - url: "http://localhost:8080"
```

</details>

<details>
<summary><b>Caddy 설정 예제</b></summary>

```caddyfile
drive.example.com {
    reverse_proxy nas.local:5001 {
        health_uri /health
        health_interval 10s
        fail_duration 30s
    }

    reverse_proxy localhost:8080
}
```

</details>

## 아키텍처

```
┌─────────────┐     ┌──────────────┐     ┌─────────────┐
│   Client    │────▶│    Proxy     │────▶│  Synology   │
└─────────────┘     │ (Traefik/    │     │    NAS      │
                    │  Caddy)      │     └─────────────┘
                    └──────┬───────┘              │
                           │                      │
                    (NAS Offline)           (Drive API)
                           │                      │
                           ▼                      ▼
                    ┌──────────────────────────────┐
                    │  synology-file-cache         │
                    │  ┌──────────┐  ┌──────────┐ │
                    │  │  Server  │  │  Syncer  │ │
                    │  └──────────┘  └──────────┘ │
                    │  ┌──────────┐  ┌──────────┐ │
                    │  │  Cacher  │  │  Store   │ │
                    │  └──────────┘  └──────────┘ │
                    └──────────────────────────────┘
```

프로젝트는 Hexagonal Architecture (포트-어댑터 패턴)를 따릅니다:

```
internal/
├── domain/     # 도메인 모델 (File, Share, Priority)
├── port/       # 인터페이스 정의
├── adapter/    # 외부 시스템 어댑터 (SQLite, Synology API, Filesystem)
├── service/    # 애플리케이션 서비스 (Syncer, Cacher, Server)
├── config/     # 설정 관리
└── logger/     # 로깅
```

## 개발

```bash
# 의존성 설치
go mod download

# 테스트
go test ./...

# 린트
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
golangci-lint run

# 포맷팅
go fmt ./...
```

## 라이선스

MIT License - [LICENSE](LICENSE) 참조

## 문의

- [GitHub Issues](https://github.com/VertexToEdge/synology-file-cache/issues)
