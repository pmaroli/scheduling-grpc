package rpc

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	pb "github.com/pmaroli/scheduling-rpc/protobufs"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var (
	host     = os.Getenv("PG_HOST")
	port     = os.Getenv("PG_PORT")
	user     = os.Getenv("PG_USER")
	password = os.Getenv("PG_PASSWORD")
	dbname   = os.Getenv("PG_DB")
)

// ReservationServer contains a sql.DB to interact with Postgres
type ReservationServer struct {
	DB *sql.DB
}

var (
	timeFormat = time.RFC3339
	emptyTime  = time.Time{}
)

// Start the gRPC server
func Start() error {
	psqlInfo := fmt.Sprintf("host=%s port=%s user=%s password=%s sslmode=disable dbname=%s ", host, port, user, password, dbname)
	fmt.Println(psqlInfo)
	db, err := sql.Open("postgres", psqlInfo)

	if err != nil {
		panic(err)
	}
	defer db.Close()

	err = db.Ping()
	if err != nil {
		panic(err)
	}
	fmt.Println("Connected to the DB!")

	// Start the gRPC server
	flag.Parse()
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", 5001))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	grpcServer := grpc.NewServer()
	pb.RegisterReservationServer(grpcServer, ReservationServer{DB: db})
	reflection.Register(grpcServer)
	return grpcServer.Serve(lis)
}

// GetAllBooks from the Postgres DB
func (s ReservationServer) GetAllBooks(ctx context.Context, req *pb.Empty) (*pb.GetAllBooksRes, error) {
	var books []*pb.Book

	getAllBooksSQL := `
		SELECT isbn, library, price, ST_Y(geog::geometry) as lat, ST_X(geog::geometry) as lng
		FROM books
	`
	rows, err := s.DB.Query(getAllBooksSQL)
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var (
			isbn    string
			library string
			lat     float32
			lng     float32
			price   float32
		)

		err = rows.Scan(&isbn, &library, &price, &lat, &lng)
		if err != nil {
			return nil, err
		}

		// Append each book to the resultant list
		books = append(books, &pb.Book{
			Isbn:    isbn,
			Lat:     lat,
			Lng:     lng,
			Price:   price,
			Library: library,
		})
	}

	fmt.Println(books)
	return &pb.GetAllBooksRes{Books: books}, nil
}

// GetBook returns a book with the matching ISBN
func (s ReservationServer) GetBook(ctx context.Context, req *pb.GetBookReq) (*pb.Book, error) {
	var (
		isbn    string
		lat     float32
		lng     float32
		price   float32
		library string

		book *pb.Book
	)

	getBookSQL := `
		SELECT isbn, library, price, ST_Y(geog::geometry) as lat, ST_X(geog::geometry) as lng
		FROM books
		WHERE isbn = $1
	`

	rows, err := s.DB.Query(getBookSQL, req.GetIsbn())
	if err != nil {
		return nil, err
	}

	if rows.Next() {
		err = rows.Scan(&isbn, &library, &price, &lat, &lng)
		if err != nil {
			return nil, err
		}

		book = &pb.Book{
			Isbn:    isbn,
			Library: library,
			Lat:     lat,
			Lng:     lng,
			Price:   price,
		}
	} else {
		return nil, errors.New("could not find book")
	}

	fmt.Println(book)
	return book, nil
}

// AddBook adds a book to the database
func (s ReservationServer) AddBook(ctx context.Context, req *pb.AddBookReq) (*pb.Empty, error) {
	newBook := req.GetBook()

	addBookSQL := `
		INSERT INTO books (isbn, library, price, geog)
		VALUES ($1, $2, $3, ST_MakePoint($4, $5))
	`

	_, err := s.DB.Exec(addBookSQL, newBook.GetIsbn(), newBook.GetLibrary(), newBook.GetPrice(), newBook.GetLng(), newBook.GetLat())
	if err != nil {
		return nil, err
	}

	// Return a status code or something?
	return &pb.Empty{}, nil
}

// ReserveBook reserves a book for a specified amount of time
func (s ReservationServer) ReserveBook(ctx context.Context, req *pb.ReserveBookReq) (*pb.Empty, error) {
	var startTime, endTime, err = parseTimes(req.GetStartDate(), req.GetEndDate())
	if err != nil {
		return nil, err
	}

	// First check if the reservation can be made
	checkReservationSQL := `
		SELECT COUNT(isbn) FROM reservations
		WHERE
			isbn = $1
		AND duration && tstzrange($2, $3)
	`
	rows, err := s.DB.Query(checkReservationSQL, req.GetIsbn(), startTime.Format(timeFormat), endTime.Format(timeFormat))
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var count int32
		err = rows.Scan(&count)
		if err != nil {
			return nil, err
		}

		if count > 0 {
			return nil, errors.New("reservation overlaps with an existing slot")
		}
	}

	// If there are no overlapping reservations, make the reservation
	reserveBookSQL := `
		INSERT INTO reservations (isbn, duration)
		VALUES ($1, tstzrange($2, $3))
	`
	_, err = s.DB.Exec(reserveBookSQL, req.GetIsbn(), startTime.Format(timeFormat), endTime.Format(timeFormat))
	if err != nil {
		return nil, err
	}

	fmt.Println(fmt.Sprintf("Made reservation for %s", req.GetIsbn()))
	// TODO: Return a status code or something?
	return &pb.Empty{}, nil
}

// CheckoutBook populates the checked_out table to signify a reservation has been 'checked out'
func (s ReservationServer) CheckoutBook(ctx context.Context, req *pb.CheckoutBookReq) (*pb.Empty, error) {
	var startTime, endTime, err = parseTimes(req.GetStartDate(), req.GetEndDate())
	if err != nil {
		return nil, err
	}

	// First get the reservation id
	// Can probably simplify this to not require the exact start/end times
	getReservationIDSQL := `
		SELECT id FROM reservations
		WHERE
			isbn = $1
			AND duration = tstzrange($2, $3)
	`
	row := s.DB.QueryRow(getReservationIDSQL, req.GetIsbn(), startTime.Format(timeFormat), endTime.Format(timeFormat))
	if err != nil {
		return nil, err
	}
	var reservationID string
	err = row.Scan(&reservationID)
	if err != nil {
		return nil, err
	}

	checkoutBookSQL := `
		INSERT INTO checked_out (isbn, reservation_id)
		VALUES ($1, $2)
	`
	_, err = s.DB.Exec(checkoutBookSQL, req.GetIsbn(), reservationID)
	if err != nil {
		// Will not allow checking out a book if the ISBN already exists in the table
		return nil, err
	}

	fmt.Println(fmt.Sprintf("Checked out book with ISBN: %s", req.GetIsbn()))
	return &pb.Empty{}, nil
}

// ReturnBook returns a previously checked out book
func (s ReservationServer) ReturnBook(ctx context.Context, req *pb.ReturnBookReq) (*pb.Empty, error) {
	returnBookSQL := `
		DELETE FROM checked_out
		WHERE isbn = $1
		RETURNING isbn
	`

	result, err := s.DB.Exec(returnBookSQL, req.GetIsbn())
	if err != nil {
		return nil, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}

	if rowsAffected == 0 {
		return nil, errors.New("book has not been checked out")
	}

	fmt.Println(fmt.Sprintf("Returned book with ISBN: %s", req.GetIsbn()))
	return &pb.Empty{}, nil
}

// DeleteBook deletes a book from the DB
func (s ReservationServer) DeleteBook(ctx context.Context, req *pb.DeleteBookReq) (*pb.Empty, error) {
	deleteBookSQL := `
		DELETE FROM books
		WHERE isbn = $1
	`

	result, err := s.DB.Exec(deleteBookSQL, req.GetIsbn())
	if err != nil {
		return nil, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}

	if rowsAffected == 0 {
		return nil, errors.New("book not found")
	}

	fmt.Println(fmt.Sprintf("Deleted book with ISBN: %s", req.GetIsbn()))
	return &pb.Empty{}, nil
}

// Search for books given the coordinates and radius of search
func (s ReservationServer) Search(ctx context.Context, req *pb.SearchReq) (*pb.SearchRes, error) {
	var startTime, endTime, err = parseTimes(req.GetStartDate(), req.GetEndDate())
	if err != nil {
		return nil, err
	}

	rangeInKm := req.GetRange()
	rangeInMeters := rangeInKm * 1000

	searchBooksSQL := `
	SELECT isbn, library, price, ST_Y(geog::geometry) as lat, ST_X(geog::geometry) as lng
	FROM books
	WHERE
		ST_DWithin(geog, ST_MakePoint($1, $2)::geography, $3)
		AND isbn NOT IN (
			SELECT DISTINCT(isbn) FROM reservations
			WHERE duration && tstzrange($4, $5)
		)
	ORDER BY geog <-> ST_MakePoint($1, $2)::geography;
	`

	rows, err := s.DB.Query(searchBooksSQL, req.GetLng(), req.GetLat(), rangeInMeters, startTime.Format(timeFormat), endTime.Format(timeFormat))
	if err != nil {
		return nil, err
	}

	var books []*pb.Book
	for rows.Next() {
		var (
			isbn    string
			library string
			lat     float32
			lng     float32
			price   float32
		)

		err = rows.Scan(&isbn, &library, &price, &lat, &lng)
		if err != nil {
			return nil, err
		}

		// Append each book to the search result
		books = append(books, &pb.Book{
			Isbn:    isbn,
			Lat:     lat,
			Lng:     lng,
			Price:   price,
			Library: library,
		})
	}

	return &pb.SearchRes{Books: books}, nil
}

func parseTimes(startTimeString, endTimeString string) (time.Time, time.Time, error) {
	if startTimeString == "" || endTimeString == "" {
		return emptyTime, emptyTime, errors.New("empty time search is not implemented right now")
	}

	var startTime, err = time.Parse(timeFormat, startTimeString)
	if err != nil {
		return emptyTime, emptyTime, errors.New("invalid datetime format: `startDate` was not formatted as ISO8601")
	}

	endTime, err := time.Parse(timeFormat, endTimeString)
	if err != nil {
		return emptyTime, emptyTime, errors.New("invalid datetime format: `endDate` was not formatted as ISO8601")
	}

	return startTime, endTime, nil
}
