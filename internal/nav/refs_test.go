package nav

import "testing"

func numRef(prefix string, id int) Ref {
	return Ref{Prefix: prefix, ID: id, Numeric: true}
}

func nameRef(prefix, name string) Ref {
	return Ref{Prefix: prefix, Name: name}
}

func assertRefs(t *testing.T, text string, want ...Ref) {
	t.Helper()
	got := ExtractRefs(text)
	if len(got) != len(want) {
		t.Fatalf("ExtractRefs() = %#v, want %#v", got, want)
	}
	count := map[Ref]int{}
	for _, r := range got {
		count[r]++
	}
	for _, r := range want {
		if count[r] == 0 {
			t.Fatalf("ExtractRefs() = %#v, missing %#v", got, r)
		}
		count[r]--
	}
}

func TestRefKey(t *testing.T) {
	key, ok := numRef("c", 12).Key()
	if !ok || key != "c12" {
		t.Fatalf("key = %s, %v", key, ok)
	}
	if _, ok := nameRef("c", "Sales-Post").Key(); ok {
		t.Fatal("name ref should not have a key")
	}
	if _, ok := (Ref{Prefix: "t", ID: 18}).Key(); ok {
		t.Fatal("non-numeric ref should not have a key")
	}
}

func TestExtractRefsTypedAndSymbolic(t *testing.T) {
	text := `
OBJECT Codeunit 11 Gen. Jnl.-Check Line
{
  PROPERTIES
  {
    TableNo=81;
    OnRun=VAR
            GenJnlLine@1000 : Record 81;
            Cust@1001 : Record Customer;
            SalesHeader@1002 : Record "Sales Header";
            PostLine@1003 : Codeunit 12;
            CustCard@1004 : Page 21;
            TopItems@1005 : Query "Top Items";
            List@1006 : Report 111;
            Port@1007 : XMLport 50000;
            OldForm@1008 : Form 50;
            Port2@1009 : Dataport 7;
            TempCust@1010 : TEMPORARY Record 18;
          BEGIN
            CODEUNIT.RUN(CODEUNIT::"Gen. Jnl.-Post Line");
            IF PAGE.RUNMODAL(PAGE::"Customer Card") = ACTION::LookupOK THEN;
            RecRef.OPEN(DATABASE::Customer);
            RecRef.OPEN(DATABASE::18);
            REPORT.RUN(REPORT::"Customer - List");
            XMLPORT.RUN(XMLPORT::"My Export");
            QUERY.RUN(QUERY::1);
            FORM.RUN(FORM::21);
            DATAPORT.RUN(DATAPORT::"Customer Export");
            CODEUNIT.RUN(CODEUNIT::Sales);
            CODEUNIT.RUN(12);
          END;
  }
}`
	assertRefs(t, text,
		numRef("t", 81),
		nameRef("t", "Customer"),
		nameRef("t", "Sales Header"),
		numRef("c", 12),
		numRef("p", 21),
		nameRef("q", "Top Items"),
		numRef("r", 111),
		numRef("x", 50000),
		numRef("f", 50),
		numRef("d", 7),
		numRef("t", 18),
		nameRef("c", "Gen. Jnl.-Post Line"),
		nameRef("p", "Customer Card"),
		nameRef("r", "Customer - List"),
		nameRef("x", "My Export"),
		numRef("q", 1),
		numRef("f", 21),
		nameRef("d", "Customer Export"),
		nameRef("c", "Sales"),
	)
}

func TestExtractRefsProperties(t *testing.T) {
	text := `
OBJECT Page 21 Customer Card
{
  PROPERTIES
  {
    SourceTable=Table18;
    LookupPageID=Page22;
    DrillDownPageID=Page23;
    CardPageID=Page24;
    PagePartID=Page50000;
  }
}
OBJECT ignored below
DataItemTable=Table15;
RunObject=Codeunit 12;
RunObject=Page 21;
LookupFormID=Form21;
SourceTable=Table 19;
TableName=81;
MyTableNo=82;
Something=83;
`
	// The second "OBJECT" line is not the file header, so "ignored below" is
	// ordinary text. TableName and MyTableNo are not TableNo.
	assertRefs(t, text,
		numRef("t", 18),
		numRef("p", 22),
		numRef("p", 23),
		numRef("p", 24),
		numRef("p", 50000),
		numRef("t", 15),
		numRef("c", 12),
		numRef("p", 21),
		numRef("f", 21),
		numRef("t", 19),
	)
}

func TestExtractRefsSkipsHeaderCommentsStringsAndPermissions(t *testing.T) {
	headerOnly := "OBJECT Codeunit 12 \"Gen. Jnl.-Post Line\"\r\n"
	assertRefs(t, headerOnly)

	pageHeader := "OBJECT Page 21 \"Customer Card\"\n"
	assertRefs(t, pageHeader)

	text := "OBJECT Codeunit 12 \"Codeunit 99\"\r\n" +
		"Permissions=TableData 81=rimd,\r\n" +
		"            TableData \"Sales Header\"=r,\r\n" +
		"            tabledata Ghost=R;\r\n" +
		"Codeunit 11 // Codeunit 12\r\n" +
		"Message('Codeunit 12');\r\n" +
		"Message('it''s // not a comment Codeunit 13');\r\n" +
		"Record \"Foo//Bar\"; Codeunit 14\r\n" +
		"// TableNo=81 TableRelation=Customer.No.\r\n" +
		"CODEUNIT::\"Say \"\"Hi\"\"\"\r\n"
	assertRefs(t, text,
		numRef("c", 11),
		nameRef("t", "Foo//Bar"),
		numRef("c", 14),
		nameRef("c", `Say "Hi"`),
	)
}

func TestExtractRefsCommentDoesNotHideNextLine(t *testing.T) {
	text := "// Message('Codeunit 12');\r\nCodeunit 11\r\n"
	assertRefs(t, text, numRef("c", 11))
}

func TestExtractRefsStringDoesNotHideRestOfLine(t *testing.T) {
	text := "Message('Codeunit 12'); Codeunit 11"
	assertRefs(t, text, numRef("c", 11))
}

func TestExtractRefsQuoteInsideComment(t *testing.T) {
	text := "// it's a 'Codeunit 12'\nCodeunit 11\n"
	assertRefs(t, text, numRef("c", 11))
}

func TestExtractRefsKeepsBodySelfReference(t *testing.T) {
	text := "OBJECT Codeunit 12 Gen. Jnl.-Post Line\nCODEUNIT.RUN(CODEUNIT::12);\n"
	assertRefs(t, text, numRef("c", 12))
}

func TestExtractRefsDedupes(t *testing.T) {
	text := "Codeunit 12\nCODEUNIT::12\nCodeunit 12\nDATABASE::Customer\ndatabase::customer\n"
	assertRefs(t, text, numRef("c", 12), nameRef("t", "Customer"))
}

func TestExtractRefsDoesNotSplitAdjacentWords(t *testing.T) {
	text := "RecordRef\nCodeunit.RUN(x)\nPagePartID\nTableRelationX\nFormat\nPerformance\n"
	assertRefs(t, text)
}

func TestExtractRefsTypeKeywordStaysOnOneLine(t *testing.T) {
	text := "Codeunit\n12\nPage\n22\n"
	assertRefs(t, text)
}

func TestExtractRefsIgnoresTooltipsTextConstAndCaptions(t *testing.T) {
	text := `
ToolTipML=ENU=Specifies the number of the XMLport that is created from this XML schema.;
CaptionML=ENU=New XMLport No.;
CaptionML=[ENU=XMLport that;
           FRB=Port XML];
OptionCaptionML=ENU=Page,Report;
OptionString=XMLport,Page;
PromotedActionCategoriesML=ENU=New,Process,Report,View;
InstructionalTextML=ENU=Open the Page that applies.;
NoObjectIDErr : TextConst 'ENU=Open Codeunit 12 now.';
Text000 : TextConst ENU=Run Page 21 later;
SourceTable=Table18;
Codeunit 12
`
	assertRefs(t, text, numRef("t", 18), numRef("c", 12))
}

func TestExtractRefsTableRelation(t *testing.T) {
	text := `
TableRelation=Customer.No.;
TableRelation="Sales Header"."No.";
TableRelation="O'Brien".Code;
TableRelation=Customer.No. WHERE (Blocked=FILTER(<>All));
TableRelation="Sales Line"."Document No." WHERE ("Document Type"=FIELD("Document Type"));
TableRelation=IF ("Account Type"=CONST("G/L Account")) "G/L Account"
               ELSE IF ("Account Type"=CONST(Customer)) Customer
               ELSE IF ("Account Type"=CONST(Vendor)) Vendor
               ELSE IF ("Account Type"=CONST("Bank Account")) "Bank Account"
               ELSE IF ("Account Type"=CONST("Fixed Asset")) "Fixed Asset"
               ELSE IF ("Account Type"=CONST("IC Partner")) "IC Partner";
TableRelation=IF (Type=CONST(" ")) "Standard Text"
               ELSE IF (Type=CONST("G/L Account")) "G/L Account"
               ELSE IF (Type=CONST(Item)) Item
               ELSE IF (Type=CONST(Resource)) Resource
               ELSE IF (Type=CONST("Fixed Asset")) "Fixed Asset"
               ELSE IF (Type=CONST("Charge (Item)")) "Item Charge";
TableRelation=IF ("Document Type"=CONST(Order)) "Sales Header"."No." WHERE ("Document Type"=CONST(Order))
               ELSE "Sales Header"."No.";
TableRelation=IF (Type=CONST(Item)) "Item Ledger Entry" ELSE Customer.No.;
TableRelation=18.No.;
`
	assertRefs(t, text,
		nameRef("t", "Customer"),
		nameRef("t", "Sales Header"),
		nameRef("t", "O'Brien"),
		nameRef("t", "Sales Line"),
		nameRef("t", "G/L Account"),
		nameRef("t", "Vendor"),
		nameRef("t", "Bank Account"),
		nameRef("t", "Fixed Asset"),
		nameRef("t", "IC Partner"),
		nameRef("t", "Standard Text"),
		nameRef("t", "Item"),
		nameRef("t", "Resource"),
		nameRef("t", "Item Charge"),
		nameRef("t", "Item Ledger Entry"),
		numRef("t", 18),
	)
}

func TestExtractRefsTableRelationInStringIsIgnored(t *testing.T) {
	text := "Message('TableRelation=Customer.No.');\nRecord 18\n"
	assertRefs(t, text, numRef("t", 18))
}

func TestExtractRefsEventSubscribers(t *testing.T) {
	text := `
[EventSubscriber(ObjectType::Codeunit, CODEUNIT::"Sales-Post", 'OnAfterPostSalesDoc', '', false, false)]
[EventSubscriber(ObjectType::Codeunit, 80, 'OnAfterPostSalesDoc', '', false, false)]
[EventSubscriber(ObjectType::Table, DATABASE::"Sales Header", 'OnAfterInsertEvent', '', false, false)]
[EventSubscriber(ObjectType::Table, 18, 'OnAfterInsertEvent', '', false, false)]
[EventSubscriber(ObjectType::Page, 21, 'OnAfterGetRecord', '', false, false)]
[EventSubscriber(ObjectType::XMLport, 50000, 'OnBeforeExport', '', false, false)]
[EventSubscriber(ObjectType::Codeunit,
    81, 'OnAfter', '', false, false)]
`
	assertRefs(t, text,
		nameRef("c", "Sales-Post"),
		numRef("c", 80),
		nameRef("t", "Sales Header"),
		numRef("t", 18),
		numRef("p", 21),
		numRef("x", 50000),
		numRef("c", 81),
	)
}

func TestExtractRefsQuotedIdentifierWithApostrophe(t *testing.T) {
	text := "Record \"O'Brien\"\n"
	assertRefs(t, text, nameRef("t", "O'Brien"))
}

func TestExtractRefsCaseAndSpacing(t *testing.T) {
	text := "tablename\ntableNo = 81\nsourceTable = table 18\nlookuppageid = page 22\nrunobject = codeunit 12\ncodeunit::\"Gen. Jnl.-Post Line\"\ndatabase::customer\n"
	assertRefs(t, text,
		numRef("t", 81),
		numRef("t", 18),
		numRef("p", 22),
		numRef("c", 12),
		nameRef("c", "Gen. Jnl.-Post Line"),
		nameRef("t", "customer"),
	)
}

func TestExtractRefsEmptyAndNoise(t *testing.T) {
	assertRefs(t, "")
	assertRefs(t, "   \n\n")
	assertRefs(t, "// only a comment")
	assertRefs(t, "'Codeunit 12'")
	assertRefs(t, "OBJECT-PROPERTIES\nCodeunit 11\n", numRef("c", 11))
	got := ExtractRefs("\uFEFFCodeunit 11\n")
	if len(got) != 1 || got[0] != numRef("c", 11) {
		t.Fatalf("bom = %#v", got)
	}
}
