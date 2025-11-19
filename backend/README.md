# WhatsApp Analysis API - Multi-User Backend

Backend untuk WhatsApp Analyzer dengan sistem multi-user, dimana setiap user memiliki session WhatsApp terpisah untuk mencegah tabrakan dan fitur reset data otomatis.

## 🚀 Fitur Utama

### 1. Multi-User Support
- ✅ **Multi-user support** dengan session isolation
- ✅ **User authentication** (register/login)
- ✅ **JWT token management**
- ✅ **WhatsApp session per user**
- ✅ **Database MySQL** dengan GORM
- ✅ **Auto migration tables**
- ✅ **CORS support**

### 2. Analisis Data WhatsApp
- **Total Chats**: Estimasi berdasarkan kontak
- **Total Contacts**: Data nyata dari WhatsApp
- **Incoming/Outgoing Messages**: Estimasi berdasarkan pola kontak
- **Two Way Chats**: Estimasi komunikasi dua arah
- **Unknown Number Chats**: Estimasi nomor tidak dikenal
- **Fast Senders**: Estimasi pengirim cepat
- **Group Activity**: Aktivitas grup
- **Total Groups**: Total grup yang diikuti
- **Total Unsaved Chats**: Chat yang tidak tersimpan
- **Total Chat with Contact**: Chat dengan kontak tersimpan
- **Account Age**: Estimasi umur akun dalam hari
- **Strength Rating**: Rating kekuatan akun

### 3. Reset Data Otomatis
✅ **Data analisis direset otomatis saat logout**
- Cache analisis dibersihkan
- Data frontend direset
- Session WhatsApp dibersihkan
- QR code baru dibuat untuk user berikutnya

### 4. Animasi Loading
- Spinner loading yang menarik
- Animasi pada tombol saat loading
- Feedback visual yang responsif

## 🏗️ Architecture

### Database Schema
```
Users
├── id (primary key)
├── username (unique)
├── email (unique)
├── password_hash
├── phone_number
├── role (admin/user)
├── is_active
└── timestamps

WhatsAppSessions
├── id (primary key)
├── user_id (foreign key)
├── session_data
├── qr_code
├── status
├── device_id
└── timestamps

AnalysisResults
├── id (primary key)
├── user_id (foreign key)
├── scan_date
├── analysis parameters (8 indikator)
├── strength rating
├── summary
└── timestamps

ScanHistory
├── id (primary key)
├── user_id (foreign key)
├── phone_number
├── scan_date
├── status
├── result_data
└── timestamps
```

### Multi-User Flow
1. **Landing Page** → Input nomor HP
2. **Check Phone** → Cek apakah user sudah register
3. **Register/Login** → Buat account atau login
4. **Dashboard** → User sudah authenticated
5. **Scan WhatsApp** → Generate QR code per user
6. **Analisis** → Analisis data WhatsApp user
7. **Save/Print** → Simpan atau print hasil
8. **Logout WA** → Disconnect WhatsApp (tetap di dashboard)

## 🛠️ Setup Instructions

### 1. Prerequisites
- Go 1.24.5+
- XAMPP (MySQL)
- Git

### 2. Database Setup
```bash
# Start XAMPP
# Create database: wa_analyzer
# Default credentials:
# Host: 127.0.0.1
# Port: 3306
# User: root
# Password: (kosong)
# Database: wa_analyzer
```

### 3. Environment Configuration
Copy `env.example` ke `.env` dan sesuaikan:
```bash
# Database Configuration
DB_TYPE=mysql
DB_HOST=127.0.0.1
DB_PORT=3306
DB_USER=root
DB_PASSWORD=
DB_NAME=wa_analyzer

# JWT Configuration
JWT_SECRET=your-super-secret-jwt-key-here-change-in-production

# Server Configuration
SERVER_PORT=9090
ENVIRONMENT=development
```

### 4. Install Dependencies
```bash
cd backend
go mod tidy
```

### 5. Run Application
```bash
# Build aplikasi
go build -o whatsapp-api main.go

# Jalankan server
./whatsapp-api

# Atau langsung run
go run main.go

# Buka browser
http://localhost:9090
```

## 📡 API Endpoints

### Authentication
- `POST /api/auth/register` - User registration
- `POST /api/auth/login` - User login
- `GET /api/auth/check-phone` - Check phone number
- `GET /api/auth/profile` - Get user profile (protected)

### WhatsApp (Per User)
- `GET /api/wa/qr` - Get QR code
- `GET /api/wa/status` - Get WhatsApp status
- `GET /api/wa/analyze` - Analyze WhatsApp data
- `POST /api/wa/logout` - Logout WhatsApp
- `POST /api/wa/qr/refresh` - Refresh QR code
- `GET /api/wa/debug` - Debug status
- `POST /api/wa/reconnect` - Manual reconnect

## 🔐 Multi-User Implementation

### Session Isolation
- Setiap user punya WhatsApp client terpisah
- Database session terpisah per user
- QR code terpisah per user
- Analisis data terpisah per user

### Security Features
- JWT token authentication
- Password hashing dengan bcrypt
- User role management
- Session timeout
- Rate limiting (TODO)

### Database Management
- GORM auto migration
- Connection pooling
- Transaction support
- Soft delete

## 🔄 Cara Kerja Reset Data

### Backend (Go)
1. **Cache Management**: Data analisis disimpan dalam cache per session
2. **Reset Function**: `ClearAnalysisCache()` membersihkan cache
3. **Logout Handler**: Memanggil reset sebelum logout
4. **Session Cleanup**: Menghapus semua data session

### Frontend (JavaScript)
1. **Status Monitoring**: Reset data saat status berubah offline
2. **Logout Handler**: Reset semua field analisis
3. **UI Reset**: Menyembunyikan hasil dan reset nilai

### Struktur Data Reset
```go
// Cache analisis
analysisData map[string]interface{}
analysisMu   sync.RWMutex

// Reset saat logout
func (w *WhatsApp) Reset() {
    // Clear analysis cache
    w.analysisMu.Lock()
    w.analysisData = make(map[string]interface{})
    w.analysisMu.Unlock()
    
    // Clear session data
    // Restart connection
}
```

### Keuntungan Reset Otomatis
1. **Privasi**: Data user sebelumnya tidak tercampur
2. **Akurasi**: Analisis selalu berdasarkan data user saat ini
3. **Konsistensi**: Hasil analisis konsisten per session
4. **Keamanan**: Tidak ada data yang tertinggal

## 📊 Development Notes

### Current Status
- ✅ Database models
- ✅ Authentication service
- ✅ User handlers
- ✅ Multi-user WhatsApp manager
- ✅ Database configuration
- ✅ Multi-user WhatsApp handler
- ✅ Analysis service per user
- ✅ Phone number check
- ✅ JWT environment configuration
- ⏳ Frontend integration
- ⏳ Save/Print functionality
- ⏳ Testing multi-user scenarios

### Next Steps
1. ✅ Integrate multi-user manager ke existing handlers
2. ✅ Update analysis system untuk multi-user
3. Implement save/print functionality
4. Add frontend routes
5. Testing multi-user scenarios

## 🐛 Troubleshooting

### Common Issues
1. **Database Connection Failed**
   - Check XAMPP MySQL service
   - Verify database credentials
   - Check firewall settings

2. **JWT Token Invalid**
   - Check JWT_SECRET in .env
   - Verify token expiration
   - Check Authorization header format

3. **WhatsApp Session Issues**
   - Clear session database files
   - Check user permissions
   - Verify device ID

### Debug Mode
Set `ENVIRONMENT=development` untuk debug logging.

## 🤝 Contributing
1. Fork repository
2. Create feature branch
3. Commit changes
4. Push to branch
5. Create Pull Request

## 📄 License
MIT License 