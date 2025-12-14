CREATE TABLE users (
    id SERIAL PRIMARY KEY,              -- Auto-incrementing unique identifier
    email VARCHAR(255) UNIQUE NOT NULL, -- Unique email address
    password_hash VARCHAR(255) NOT NULL, -- Bcrypt hashed password
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Optional: Create an index on email for faster lookups
CREATE INDEX idx_users_email ON users(email);