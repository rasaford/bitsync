package meta

// var tests []file.testFile = []file.testFile{{
// 	"../testData/testFile",
// 	8054,
// 	// shasum testData/testFile | tr "[a-z]" "[A-Z]"
// 	"BC6314A1D1D36EC6C0888AF9DBD3B5E826612ADA",
// 	// dd if=testData/testFile bs=25 count=1 | shasum | tr "[a-z]" "[A-Z]"
// 	"F072A5A05C7ED8EECFFB6524FBFA89CA725A66C3",
// 	// dd if=testData/testFile bs=25 count=1 skip=1 | shasum | tr "[a-z]" "[A-Z]"
// 	"859CF11E055E61296F42EEB5BB19E598626A5173",
// }}

// func TestComputeSums(t *testing.T) {
// 	pieceLen := int64(25)
// 	for _, testFile := range tests {
// 		fs, err := mkFileStore(testFile)
// 		if err != nil {
// 			t.Fatal(err)
// 		}
// 		sums, err := computeSums(fs, testFile.fileLen, pieceLen)
// 		if err != nil {
// 			t.Fatal(err)
// 		}
// 		if len(sums) < sha1.Size {
// 			t.Errorf("computeSums got len %d, wanted %d", len(sums), sha1.Size)
// 		}
// 		a := fmt.Sprintf("%X", sums[:sha1.Size])
// 		b := fmt.Sprintf("%X", sums[sha1.Size:sha1.Size*2])
// 		if a != testFile.hashPieceA {
// 			t.Errorf("Piece A Wanted %v, got %v\n", testFile.hashPieceA, a)
// 		}
// 		if b != testFile.hashPieceB {
// 			t.Errorf("Piece B Wanted %v, got %v\n", testFile.hashPieceB, b)
// 		}
// 	}
// }

// func mkFileStore(tf file.testFile) (fs *file.fileStore, err error) {
// 	f := fileEntry{tf.fileLen, &osFile{tf.path}}
// 	return &fileStore{fileSystem: nil, offsets: []int64{0}, files: []fileEntry{f}, pieceSize: 512}, nil
// }
