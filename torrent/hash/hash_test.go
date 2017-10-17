package hash

import (
	"io"
	"strings"
	"testing"
)

func TestFileHash(t *testing.T) {
	type args struct {
		file io.Reader
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			"string hash",
			args{strings.NewReader("this is a test")},
			"VLDFjHzp8qi1UTURAu4JOA==",
		},
		{
			"longer string hash",
			args{strings.NewReader("Lorem ipsum dolor sit amet, consectetur adipiscing elit. Vestibulum ac ultrices nunc. In gravida nulla sed bibendum congue. Sed nisi massa, placerat in venenatis et, imperdiet nec nunc. Donec eget condimentum lacus. Donec volutpat tristique tellus, vel placerat odio auctor eget. Pellentesque et felis vestibulum, venenatis erat vel, ullamcorper metus. Aliquam vestibulum arcu vitae felis maximus, a dapibus nisl euismod. Morbi interdum, tortor non lobortis eleifend, justo sem cursus sem, vel bibendum lacus diam et neque. Sed ante mauris, congue et eleifend at, aliquet vel est. Cras ornare, dolor quis sagittis auctor, nibh massa eleifend velit, sit amet sollicitudin sapien nisi quis sapien. Suspendisse fermentum, mauris in efficitur ornare, turpis elit bibendum tellus, tristique cursus augue odio ut elit. Nullam malesuada mauris purus, finibus dapibus nibh congue vitae. Fusce tincidunt pharetra faucibus. Fusce quis feugiat felis. Vestibulum dignissim malesuada massa, nec tincidunt nulla tincidunt eget. Orci varius natoque penatibus et magnis dis parturient montes, nascetur ridiculus mus. Integer a sollicitudin ante, ac condimentum dolor. Etiam nisl orci, ultricies a vehicula ac, condimentum ut lorem. Integer vel ex imperdiet, scelerisque velit ac, porttitor nulla. In sit amet arcu eget odio commodo tincidunt. Nulla ornare ipsum ac lacus faucibus consequat. Donec quis ligula pellentesque, cursus quam id, sodales ante. Integer eget ex feugiat, ultrices elit pretium, dictum est. Suspendisse condimentum nibh ac orci feugiat, elementum accumsan ex laoreet. Donec pretium nisl et sollicitudin eleifend. Mauris gravida rutrum lobortis. Integer sed lectus nec dolor rhoncus tempor porttitor ac lectus. Morbi diam magna, commodo id volutpat consequat, gravida eget turpis. Interdum et malesuada fames ac ante ipsum primis in faucibus. Morbi ut ultrices mi. Pellentesque mollis magna augue, eu cursus nibh eleifend luctus. Etiam dictum, orci id sollicitudin pulvinar, dui magna molestie tellus, id vulputate velit erat eu nibh. In iaculis ante sit amet turpis convallis, a egestas est pulvinar. Aliquam iaculis hendrerit justo, vel mattis sem congue vitae. Quisque accumsan turpis tincidunt ipsum commodo aliquet. Etiam ultricies turpis et sem ornare porttitor. Etiam cursus quam quis aliquam maximus. In sed nulla et felis porta interdum. Nunc diam elit, placerat eget sagittis vitae, rhoncus eleifend enim. Sed in condimentum nulla, eget rhoncus ligula. Nam ut vulputate sapien. Praesent in urna consectetur, vulputate elit vitae, condimentum purus. Morbi tellus est, dignissim eu nibh a, luctus suscipit sem. Vestibulum commodo ligula quis lectus facilisis, vitae placerat ante pretium. Duis placerat faucibus convallis. Cras vitae nibh tortor. Orci varius natoque penatibus et magnis dis parturient montes, nascetur ridiculus mus. Mauris nec ipsum nec libero ullamcorper suscipit lacinia eget nisl. Nam blandit, mi vel luctus venenatis, mi risus laoreet orci, vel ultrices leo nisl sit amet odio. Mauris dignissim maximus felis, viverra scelerisque nunc vulputate eu. Nulla tellus enim, placerat at efficitur eu, varius et eros.")},
			"MuXy0UleW79Sp5uRuwE66g==",
		},
		{
			"empty string test",
			args{strings.NewReader("")},
			"1B2M2Y8AsgTpgAmY7PhCfg==",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FileHash(tt.args.file); got != tt.want {
				t.Errorf("FileHash() = %v, want %v", got, tt.want)
			}
		})
	}
}
