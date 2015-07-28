use strict;
open(F, '<' . $ARGV[0]) || die;

my $rsParity = 0;
my $inQHexChar = 0;
my $inString = 0;
my $leadingZero = 0;
my $inInt = 0;

while ($_ = <F>) {
    my $line = $_;
    my $len = length($line);
    my $idx = 0;
    while ($idx < $len) {
        $_ = substr($line, $idx, 1);
        $idx++;
        if ($inQHexChar) {
            die unless (/[0-9a-f]/);
            print substr("1234567890abcdef", int(rand(16)), 1);
            if ($inQHexChar == 4) {
                $inQHexChar = 0;
            } else {
                $inQHexChar++;
            }
            next;
        }
        if (!$inString) {
            die if (/[\\\tA-Z]/);
            if (!$inInt) {
                if (/[0]/) {
                    die if ($leadingZero);
                    $leadingZero = 1;
                    print $_;
                    next;
                }
                $leadingZero = 0;
                $inInt = 1;
                if (/[1-9]/) {
                    print substr("123456789", int(rand(9)), 1);
                    next;
                }
            } elsif (/[0-9]/) {
                die if ($leadingZero);
                print substr("1234567890", int(rand(10)), 1);
                next;
            }
            $inInt = 0;
            $leadingZero = 0;
            if (/["]/) {
                $inString = 1;
                print $_;
                next;
            }
            print $_;
            next;
        }
        if (/[\\]/) {
            $rsParity = ($rsParity + 1) % 2;
            print $_;
            next;
        }
        if ($rsParity) {
            $rsParity = 0;
            if (/["\/]/) {
                print $_;
                next;
            }
            if (/[u]/) {
                $inQHexChar = 1;
                print $_;
                next;
            }
            print '\\';
            next;
        }
        if (/["]/) {
            $inString = 0;
            print $_;
            next;
        }
        if (/[0-9]/) {
            print substr("1234567890", int(rand(10)), 1);
            next;
        }
        if (/[a-z \t]/) {
            print substr("abcdefghijklmnopqrstuvwxyz ", int(rand(27)), 1);
            next;
        }
        if (/[A-Z]/) {
            print substr("ABCDEFGHIJKLMNOPQRSTUVWXYZ ", int(rand(27)), 1);
            next;
        }
        print $_;
        next;
    }
}

close(F);