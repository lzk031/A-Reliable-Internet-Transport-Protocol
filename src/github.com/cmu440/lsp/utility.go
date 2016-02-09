package lsp

import (
	"container/list"
	"fmt"
)

const (
	IS_DEBUG = false
)

// this method will insert a message
// into the right place of a list sored
// based on the SeqNum of message
func InsertIntoList(l *list.List, mes *Message) {
	if l.Front() == nil {
		l.PushBack(mes)
		return
	}
	e := l.Back()
	for ; e != nil; e = e.Prev() {
		if e.Value.(*Message).SeqNum < mes.SeqNum {
			break
		}
	}
	if e == nil {
		l.PushFront(mes)
	} else {
		l.InsertAfter(mes, e)
	}
}

// this method takes in a list
// and print all the elements
// in format
func PrintMesList(l *list.List) {
	front := l.Front()

	fmt.Print("[")
	for front != nil {
		fmt.Print(front.Value)
		front = front.Next()
		if front != nil {
			fmt.Print(",")
		}
	}
	fmt.Println("]")

}

func Log(tag string, id int, message string) {
	if IS_DEBUG {
		fmt.Printf("Tag: %s, id: %d, %s\n", tag, id, message)
	}
}

func Hash(payload []byte) (hash []byte) {
	return nil
}
