CREATE TABLE categories (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    user_id INT NOT NULL,       -- Optional: categories per user
    company_id INT NOT NULL     -- Each company has its own categories
);
