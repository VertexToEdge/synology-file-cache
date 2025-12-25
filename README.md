# Synology File Cache

Synology Drive HTTP API를 사용해 중요 파일들을 로컬에 프리패치하고, NAS 오프라인 시에도 파일 서빙이 가능하도록 하는 Go 기반 캐싱 서비스입니다.

## 주요 기능

- **우선순위 기반 프리패치**: 공유/즐겨찾기/라벨/최근 수정 파일을 자동으로 로컬에 캐싱
- **스마트 디스크 관리**: 설정 가능한 디스크 사용량 제한과 우선순위+LRU 기반 자동 삭제
- **Synology 호환성**: Synology 공유 링크 토큰을 그대로 사용하여 기존 링크 유지
- **자동 캐시 갱신**: 파일 수정 시간(mtime) 기반 자동 캐시 무효화
- **라벨 제외 설정**: 특정 라벨이 붙은 파일은 캐싱에서 제외 가능
- **투명한 프록시**: NAS 온라인 시에는 직접 접근, 오프라인 시에는 캐시에서 서빙

## 아키텍처

```
┌─────────────┐     ┌──────────────┐     ┌─────────────┐
│   Client    │────▶│ Proxy        │────▶│  Synology   │
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
                    │  │ HTTP API │  │  Syncer  │ │
                    │  └──────────┘  └──────────┘ │
                    │  ┌──────────┐  ┌──────────┐ │
                    │  │  Cacher  │  │  Store   │ │
                    │  └──────────┘  └──────────┘ │
                    │         │           │        │
                    │         ▼           ▼        │
                    │  ┌──────────┐  ┌──────────┐ │
                    │  │Local FS  │  │ SQLite   │ │
                    │  └──────────┘  └──────────┘ │
                    └──────────────────────────────┘
```

### 주요 컴포넌트

- **HTTP API**: 파일 다운로드 및 디버깅 엔드포인트 제공
- **DriveSyncer**: Synology Drive API와 주기적 동기화 (풀 스캔/증분)
- **Cacher**: 우선순위 기반 프리패치 및 LRU eviction 관리
- **Store**: SQLite 기반 메타데이터 및 상태 관리
- **FS Manager**: 로컬 파일시스템 관리 및 디스크 사용량 모니터링
- **Scanner**: 즐겨찾기/라벨 폴더 재귀 스캔

## 설치

### 요구사항

- Go 1.21 이상
- SQLite3
- Linux/macOS (Windows는 WSL2 권장)

### 빌드

```bash
# 저장소 클론
git clone https://github.com/VertexToEdge/synology-file-cache.git
cd synology-file-cache

# 의존성 설치 및 빌드
go mod download
go build -o synology-file-cache ./cmd/synology-file-cache
```

## 설정

`config.yaml.example`을 복사하여 `config.yaml`로 설정합니다:

```bash
cp config.yaml.example config.yaml
```

```yaml
# Synology NAS 연결 설정
synology:
  base_url: "https://nas.local:5001"  # Synology Drive Server URL
  username: "admin"                    # 관리자 계정
  password: "password"                 # 비밀번호
  skip_tls_verify: false              # 자체 서명 인증서 사용 시 true

# 캐시 설정
cache:
  root_dir: "/var/lib/synology-file-cache"  # 캐시 저장 경로
  max_size_gb: 50                           # 최대 캐시 크기 (GB)
  max_disk_usage_percent: 50                # 디스크 사용률 제한 (%)
  recent_modified_days: 30                  # 최근 수정 파일 기준 (일)
  recent_accessed_days: 30                  # 최근 접근 파일 기준 (일)

# 동기화 설정
sync:
  full_scan_interval: "1h"        # 전체 스캔 주기
  incremental_interval: "1m"      # 증분 동기화 주기
  prefetch_interval: "30s"        # 프리패치 실행 주기
  exclude_labels: []              # 캐싱 제외할 라벨 (예: ["임시", "no-cache"])

# HTTP 서버 설정
http:
  bind_addr: "0.0.0.0:8080"      # 서비스 바인딩 주소

# 로깅 설정
logging:
  level: "info"                  # debug, info, warn, error
  format: "json"                 # json 또는 text
```

### 캐시 우선순위

파일은 다음 우선순위로 캐싱됩니다 (낮은 숫자 = 높은 우선순위):

| 우선순위 | 유형 | 설명 |
|---------|------|------|
| 1 | 공유된 파일 | 외부 공유 링크가 있는 파일 |
| 2 | 즐겨찾기/라벨 | Star 표시된 파일 또는 라벨이 붙은 파일 |
| 3 | 최근 수정 | 설정된 기간 내 수정된 파일 |
| 4 | 최근 접근 | 설정된 기간 내 접근된 파일 (예약) |
| 5 | 기본값 | 기타 파일 |

**캐싱 순서**: 우선순위 오름차순 → 파일 크기 오름차순
**삭제 순서**: 우선순위 내림차순 → LRU (가장 오래 접근 안 된 파일 먼저)

### 캐시 무효화

파일이 NAS에서 수정되면 자동으로 캐시가 무효화됩니다:
1. Syncer가 파일의 mtime(수정 시간) 변경 감지
2. 기존 캐시를 무효화 (`cached = false`)
3. 다음 Cacher 루프에서 자동으로 새 버전 다운로드

## 실행

### 기본 실행

```bash
./synology-file-cache -config config.yaml
```

### systemd 서비스 (Linux)

`/etc/systemd/system/synology-file-cache.service`:

```ini
[Unit]
Description=Synology File Cache Service
After=network.target

[Service]
Type=simple
User=synology-cache
Group=synology-cache
WorkingDirectory=/opt/synology-file-cache
ExecStart=/opt/synology-file-cache/synology-file-cache -config /etc/synology-file-cache/config.yaml
Restart=on-failure
RestartSec=10

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl enable synology-file-cache
sudo systemctl start synology-file-cache
```

## API 엔드포인트

### 헬스체크
```bash
GET /health
```
서비스 상태를 확인합니다.

### 파일 다운로드
```bash
GET /f/{token}              # permanent_link 토큰으로 다운로드
GET /d/s/{token}            # Synology 형식 호환
GET /d/s/{token}/{filename} # 파일명 포함 경로
```
Synology 공유 토큰으로 파일을 다운로드합니다.

### 디버깅

```bash
GET /debug/stats   # 캐시 통계 (JSON)
GET /debug/files   # 캐시된 파일 목록 (JSON)
```

## 프록시 설정

### Traefik 예제

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

### Caddy 예제

```caddyfile
drive.example.com {
    @nasOnline {
        not {
            path /f/*
        }
    }

    reverse_proxy @nasOnline nas.local:5001 {
        health_uri /health
        health_interval 10s
        fail_duration 30s
    }

    reverse_proxy localhost:8080
}
```

## 개발 현황

### ✅ 구현 완료

- 설정 파일 로딩 및 검증 (config)
- 구조화된 로깅 (logger)
- SQLite 데이터베이스 (store)
- HTTP API 서버 (httpapi)
- Synology Drive API 클라이언트 (synoapi)
- 동기화 엔진 - DriveSyncer (syncer)
  - Full sync / Incremental sync
  - 공유, 즐겨찾기, 라벨, 최근 수정 파일 동기화
  - mtime 기반 캐시 무효화
  - 라벨 제외 설정
- 캐싱 엔진 (cacher)
  - 우선순위 기반 프리패치
  - LRU eviction with rate limiting
- 로컬 파일시스템 관리 (fs)
  - Atomic 파일 쓰기
  - 디스크 사용량 모니터링
- 폴더 재귀 스캔 (scanner)
- Graceful shutdown

### 📋 TODO

- [ ] HTTP Range 요청 기반 이어받기 (Resume) 기능
  - 다운로드 중단 시 이어서 받기 지원
  - 임시 파일 크기 확인 및 Range 헤더 사용
  - 대용량 파일 다운로드 안정성 향상
- [ ] 메트릭 수집 및 노출 (Prometheus)
- [ ] Docker 이미지 빌드
- [ ] 통합 테스트 작성
- [ ] 비밀번호 보호 공유 링크 처리
- [ ] 공유 링크 만료 처리

## 기여하기

### 개발 환경 설정

```bash
# 개발 의존성 설치
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# 테스트 실행
go test ./...

# 린트 검사
golangci-lint run

# 코드 포맷팅
go fmt ./...
```

### 코드 구조

```
.
├── cmd/                        # 실행 파일 엔트리포인트
│   └── synology-file-cache/
├── internal/                   # 내부 패키지
│   ├── config/                # 설정 관리
│   ├── logger/                # 로깅
│   ├── store/                 # 데이터베이스
│   ├── fs/                    # 파일시스템
│   ├── synoapi/              # Synology API 클라이언트
│   ├── syncer/               # 동기화 엔진
│   ├── scanner/              # 폴더 스캐너
│   ├── cacher/               # 캐싱 엔진
│   └── httpapi/              # HTTP API
├── config.yaml.example        # 설정 파일 예제
├── CLAUDE.md                  # Claude Code 가이드
└── README.md
```

## 라이선스

MIT License - 자세한 내용은 [LICENSE](LICENSE) 파일을 참조하세요.

## 문의 및 지원

- Issue Tracker: [GitHub Issues](https://github.com/VertexToEdge/synology-file-cache/issues)
