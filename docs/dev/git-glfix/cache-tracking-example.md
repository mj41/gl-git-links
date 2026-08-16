
## Commits

commit c1
- file-d1.txt - add content lines L1 to L10
- file-s1.txt - link i1 to file-d1.txt#L5

cache after c1
- file-d1.txt - start to track diff i1:c1:oL5:0
- commits order: c1
- links: file-s1.txt:i1

commit c2
- file-s2.txt - link i2 to file-d1.txt#L7

cache after c2
- file-d1.txt - track diff i1:c1:oL5:0, start to track diff i2:c2:oL7:0
- commits order: c1, c2
- links: file-s1.txt:i1, file-s2.txt:i2

commit c3
- file-s1.txt - link i1 changed to file-d1.txt#L8

cache after c3
- file-d1.txt - track diff i2:c2:oL7:0, i1:c3:oL8:0
- commits order: c2, c3
- links: file-s1.txt:i1, file-s2.txt:i2

commit c4
- file-d1.txt - insert 3 lines at line L4

cache after c4
- file-d1.txt - track diff i2:c2:oL7:+3, i1:c3:oL8:+3
- commits order: c2, c3
- links: file-s1.txt:i1, file-s2.txt:i2

commit c5
- file-d1.txt - delete lines L5 and L6

cache after c5
- file-d1.txt - track diff i2:c2:oL7:+1, i1:c3:oL8:+1
- commits order: c2, c3
- links: file-s1.txt:i1, file-s2.txt:i2

commit c6
- file-d1.txt - modify line L4 95%

cache after c6
- file-d1.txt - track diff i2:c2:oL7:+1, i1:c3:oL8:+1
- commits order: c2, c3
- links: file-s1.txt:i1, file-s2.txt:i2

commit c7
- file-d1.txt - change line L8 (originally oL7) 42%

cache after c7
- file-d1.txt - track diff i2:c2:oL7:+1, i1:c3:oL8:+1, modifications: i2:c2:oL7:c7:42%
  - note: oL7 is original line L7 in c2, now line L8 in c7 (as there is L1-L6:+1 diff tracked)
- commits order: c2, c3
- links: file-s1.txt:i1, file-s2.txt:i2

tool run on file-s1.txt and file-s2.txt should update:
- file-s1.txt, link i1 from file-d1.txt#L8 to file-d1.txt#L9
- file-s2.txt, link i2 from file-d1.txt#L7 to file-d1.txt#L8
  - showing warning (based on i2:c2:oL7:c7:42%) that line oL7 in c2 was modified 42% in c7

commit c8
- file-s2.txt - link i2 changed to file-d1.txt#L8

cache after c8
- file-d1.txt - track diff i1:c3:oL8:+1, i2:c8:oL8:0, modifications: -
- commits order: c3, c8
- links: file-s1.txt:i1, file-s2.txt:i2
