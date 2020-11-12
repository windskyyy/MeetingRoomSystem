package model

type NotSignLink struct {
	roomId string
	meetStartTime string
	prev *NotSignLink
	next *NotSignLink
}

type SignLink struct {
	roomId string
	meetStartTime string
	prev *SignLink
	next *SignLink
}

func NewNotSign() *NotSignLink {
	return &NotSignLink{
		"",
		"",
		nil,
		nil,
	}
}

func NewSign() *SignLink {
	return &SignLink{
		"",
		"",
		nil,
		nil,
	}
}

func (node *NotSignLink) insertNode(head *NotSignLink) {
	node.next = head.next
	if head.next != nil {
		head.next.prev = node
	}
	node.prev = head
	head.next = node
}

func (node *SignLink) insertNode(head *SignLink) {
	node.next = head.next
	if head.next != nil {
		head.next.prev = node
	}
	node.prev = head
	head.next = node
}

func (node *NotSignLink) delNode() {
	if node.prev != nil {
		node.prev.next = node.next
	}
	if node.next != nil {
		node.next.prev = node.prev
	}
}

func (node *SignLink) delNode() {
	if node.prev != nil {
		node.prev.next = node.next
	}
	if node.next != nil {
		node.next.prev = node.prev
	}
}