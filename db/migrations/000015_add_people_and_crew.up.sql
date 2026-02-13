-- Add people management, instructor tracking, and multi-crew time fields

-- Contacts table: reusable people that a user can reference across flights
CREATE TABLE contacts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    email VARCHAR(255),
    phone VARCHAR(50),
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_contacts_user_id ON contacts(user_id);
CREATE INDEX idx_contacts_name ON contacts(user_id, LOWER(name));

-- Crew role enum
CREATE TYPE crew_role AS ENUM ('PIC', 'SIC', 'Instructor', 'Student', 'Passenger', 'SafetyPilot', 'Examiner');

-- Flight crew members join table
CREATE TABLE flight_crew_members (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    flight_id UUID NOT NULL REFERENCES flights(id) ON DELETE CASCADE,
    contact_id UUID REFERENCES contacts(id) ON DELETE SET NULL,
    name VARCHAR(255) NOT NULL,
    role crew_role NOT NULL
);
CREATE INDEX idx_flight_crew_flight_id ON flight_crew_members(flight_id);
CREATE INDEX idx_flight_crew_contact_id ON flight_crew_members(contact_id);

-- Extended flight fields for instructor, comments, and multi-crew times
ALTER TABLE flights ADD COLUMN instructor_name VARCHAR(255);
ALTER TABLE flights ADD COLUMN instructor_comments TEXT;
ALTER TABLE flights ADD COLUMN pilot_comments TEXT;
ALTER TABLE flights ADD COLUMN sic_time DECIMAL(5,2) NOT NULL DEFAULT 0 CHECK (sic_time >= 0);
ALTER TABLE flights ADD COLUMN dual_given_time DECIMAL(5,2) NOT NULL DEFAULT 0 CHECK (dual_given_time >= 0);
ALTER TABLE flights ADD COLUMN simulated_flight_time DECIMAL(5,2) NOT NULL DEFAULT 0 CHECK (simulated_flight_time >= 0);
ALTER TABLE flights ADD COLUMN ground_training_time DECIMAL(5,2) NOT NULL DEFAULT 0 CHECK (ground_training_time >= 0);
