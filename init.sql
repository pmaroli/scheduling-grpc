CREATE EXTENSION postgis;
CREATE EXTENSION btree_gist;

CREATE TABLE books (
    isbn VARCHAR PRIMARY KEY NOT NULL,
    library VARCHAR,
    price FLOAT8,
    geog GEOGRAPHY
);

CREATE TABLE reservations (
    id SERIAL UNIQUE,
    isbn VARCHAR REFERENCES books (isbn),
    duration TSTZRANGE,
    EXCLUDE USING gist (isbn WITH =, duration WITH &&)
);

CREATE TABLE checked_out (
    isbn VARCHAR UNIQUE REFERENCES books (isbn),
    reservation_id INT REFERENCES reservations (id)
);

CREATE INDEX reservation_index ON reservations USING gist (duration);
CREATE INDEX geograph_index ON books USING gist (geog);

INSERT INTO books (isbn, library, price, geog)
VALUES
    (9917, 'Newport Beach', 50.6, ST_MakePoint(-117.9298, 33.6189)),
    (1245, 'Newport Beach', 500.50, ST_MakePoint(-117.9298, 33.6189)),
    (1351, 'Irvine', 25, ST_MakePoint(-117.8265, 33.6846)),
    (5232, 'Costa Mesa', 300, ST_MakePoint(-117.9047, 33.6638));