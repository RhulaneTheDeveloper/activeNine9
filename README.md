# Street2Screen - User Management System

A complete Go backend with MySQL database for user management with full CRUD operations.

## Features

- MySQL database with Docker Compose setup
- User management with roles (buyer, entrepreneur, admin)
- Password hashing with bcrypt
- RESTful API endpoints
- Input validation and prepared statements
- Default users creation on startup

## Quick Start

### 1. Start MySQL Database

```bash
docker-compose up -d
```

This will start MySQL on port 3306 with:
- Database: `street2screen`
- User: `appuser`
- Password: `apppass123`

### 2. Set Environment Variables

```bash
export DB_USER=appuser
export DB_PASS=apppass123
export DB_HOST=127.0.0.1
export DB_PORT=3306
export DB_NAME=street2screen
```

### 3. Install Dependencies and Run

```bash
go mod tidy
go run main.go
```

The server will start on `http://localhost:8080`

## API Endpoints

### Create User
```bash
curl -X POST http://localhost:8080/users \
  -H "Content-Type: application/json" \
  -d '{"username":"alice","email":"alice@example.com","role":"buyer","password":"supersecret"}'
```

### List Users
```bash
curl http://localhost:8080/users
```

### Get User
```bash
curl http://localhost:8080/users/1
```

### Update User
```bash
curl -X PUT http://localhost:8080/users/1 \
  -H "Content-Type: application/json" \
  -d '{"email":"new@example.com","role":"entrepreneur"}'
```

### Delete User
```bash
curl -X DELETE http://localhost:8080/users/1
```

## Default Users

The application creates these default users on startup:
- **buyer** / buyerpass123 (buyer@example.com)
- **entrepreneur** / entpass123 (entrepreneur@example.com)
- **admin** / adminpass123 (admin@example.com)

## Security Notes

- Passwords are hashed with bcrypt before storage
- All database queries use prepared statements
- Input validation for user roles
- Environment variables for database configuration

## Production Considerations

- Change default passwords
- Use TLS/HTTPS in production
- Add authentication and authorization
- Implement rate limiting
- Add comprehensive logging
- Use proper secret management
